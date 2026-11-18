package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/webdav"

	"github.com/vedantlavale/taasant/tgstore"
)

// drive shows the index as a disk with folders. It has the five methods the
// webdav package asks for: Mkdir, OpenFile, RemoveAll, Rename and Stat.
//
// A file's name in the index is its whole path on the drive, such as
// "photos/a.jpg". The top of the drive has the name "".
type drive struct {
	store     *tgstore.Store
	key       []byte
	indexFile string
	since     time.Time
	// note is told what the drive did. alert is told that a file was not
	// saved, which Finder keeps to itself.
	note  func(text string)
	alert func(text string)

	progress transfers

	// Finder asks for many things at once, so mu guards everything below.
	mu sync.Mutex
	// files is the index as it was last read, and read is the time the
	// index file had then.
	files map[string]entry
	read  time.Time
	// local holds what is never uploaded: Finder's helper files and files
	// that are still empty. It is forgotten when the drive is ejected.
	local map[string]kept
	// folders are the folders made here. A folder is not stored anywhere:
	// apart from these, it is there for as long as a file is inside it.
	folders map[string]bool
	// changed says the index needs a new backup in the chat.
	changed bool
}

type kept struct {
	data []byte
	when time.Time
}

func newDrive(store *tgstore.Store, key []byte, indexFile string) *drive {
	return &drive{
		store: store, key: key, indexFile: indexFile, since: time.Now(),
		note: func(string) {}, alert: func(string) {},
		local: map[string]kept{}, folders: map[string]bool{},
	}
}

// sizeKey is where a request's context keeps how big the file being sent is.
type sizeKey struct{}

// bodyKey is where a request's context keeps the reader the body was read
// from. A copy that breaks off can fail before the drive sees a single byte,
// and the request's own error does not always reach the handler in time.
type bodyKey struct{}

type observedBody struct {
	io.ReadCloser
	err error
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.err = err
	}
	return n, err
}

// handler answers Finder. Requests under any other host name are refused: a
// web page can reach this computer under a name of its own, and must not get
// at the files that way.
func (d *drive) handler(host string) http.Handler {
	dav := &webdav.Handler{
		Prefix:     "/" + driveName,
		FileSystem: d,
		LockSystem: newLocks(),
		Logger: func(r *http.Request, err error) {
			// Finder asks for many files that are not there, and a file
			// that could not be saved has its own alert.
			if err != nil && !errors.Is(err, fs.ErrNotExist) && r.Method != http.MethodPut {
				d.note(fmt.Sprintf("%s %s: %v", r.Method, clean(strings.TrimPrefix(r.URL.Path, "/"+driveName)), err))
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "the taasant drive only answers Finder on this computer", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPut {
			body := &observedBody{ReadCloser: r.Body}
			r.Body = body
			ctx := context.WithValue(r.Context(), bodyKey{}, body)
			if r.ContentLength > 0 {
				ctx = context.WithValue(ctx, sizeKey{}, r.ContentLength)
			}
			r = r.WithContext(ctx)
		}
		if r.Method == http.MethodGet {
			if err := d.reach(r); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		// These three are only passed on for a name that is there. After a
		// save that failed, the name is not, and the webdav package would
		// then do harm: for a lock it makes an empty file, which would pass
		// for the saved one. And when a file is moved over another, it
		// deletes the other first, before it finds nothing to move.
		if r.Method == "LOCK" || r.Method == "MOVE" || r.Method == "COPY" {
			if _, err := d.Stat(r.Context(), strings.TrimPrefix(r.URL.Path, "/"+driveName)); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		// Finder moves a file over another without saying that it may,
		// which by the rules of WebDAV means yes. The webdav package takes
		// it for no, and saving by "write a new file, move it over the old
		// one" would fail.
		if r.Method == "MOVE" && r.Header.Get("Overwrite") == "" {
			r.Header.Set("Overwrite", "T")
		}
		dav.ServeHTTP(w, r)
	})
}

// reach fetches the first byte a download asks for, before the download is
// answered. Finder only takes a download for failed if the answer says so.
// Once bytes are on their way, it takes whatever arrives for the whole file.
func (d *drive) reach(r *http.Request) error {
	name := clean(strings.TrimPrefix(r.URL.Path, "/"+driveName))
	d.mu.Lock()
	info, err := d.find(name)
	d.mu.Unlock()
	if err != nil || info.id == "" {
		return nil
	}

	file := d.store.Open(r.Context(), d.key, info.id, info.size)
	file.Seek(firstByte(r.Header.Get("Range"), info.size), io.SeekStart)
	if _, err := file.Read(make([]byte, 1)); err != nil && err != io.EOF {
		d.note(fmt.Sprintf("%s could not be read: %v", name, err))
		return err
	}
	return nil
}

// firstByte returns where a download starts. Finder asks for a whole file
// with no Range header, and for a piece of one with "bytes=100-199".
func firstByte(header string, size int64) int64 {
	pieces, found := strings.CutPrefix(header, "bytes=")
	if !found {
		return 0
	}
	from, to, _ := strings.Cut(pieces, "-")
	if from == "" {
		// "bytes=-50" is the last 50 bytes.
		last, _ := strconv.ParseInt(to, 10, 64)
		return max(size-last, 0)
	}
	start, _ := strconv.ParseInt(from, 10, 64)
	return start
}

// clean turns the path of a request into a name: no slash at either end.
func clean(name string) string {
	return strings.Trim(path.Clean("/"+name), "/")
}

// parent returns the folder a name is in.
func parent(name string) string {
	if dir := path.Dir(name); dir != "." {
		return dir
	}
	return ""
}

// under reports whether name is folder or something inside it.
func under(name, folder string) bool {
	return name == folder || strings.HasPrefix(name, folder+"/")
}

// helper reports whether name is a file Finder writes for itself.
func helper(name string) bool {
	base := path.Base(name)
	return base == ".DS_Store" || strings.HasPrefix(base, "._")
}

// stored returns the files in the index by name. Of two with one name it
// keeps the newer. The index file is read again only after it has changed,
// which a taasant command in another window can do at any time.
func (d *drive) stored() (map[string]entry, error) {
	info, err := os.Stat(d.indexFile)
	if err == nil && d.files != nil && info.ModTime().Equal(d.read) {
		return d.files, nil
	}
	entries, err := loadIndex(d.indexFile)
	if err != nil {
		return nil, err
	}
	d.files, d.read = map[string]entry{}, time.Time{}
	if info != nil {
		d.read = info.ModTime()
	}
	for _, e := range entries {
		if old, found := d.files[e.Name]; !found || e.Uploaded.After(old.Uploaded) {
			d.files[e.Name] = e
		}
	}
	return d.files, nil
}

// update changes the index file.
func (d *drive) update(change func([]entry) []entry) error {
	entries, err := loadIndex(d.indexFile)
	if err != nil {
		return err
	}
	if err := saveIndex(d.indexFile, change(entries)); err != nil {
		return err
	}
	d.files, d.changed = nil, true
	return nil
}

// find looks a name up: among the local files, then the stored ones, then
// the folders.
func (d *drive) find(name string) (fileInfo, error) {
	folder := fileInfo{name: name, when: d.since, dir: true}
	if name == "" {
		return folder, nil
	}
	if file, found := d.local[name]; found {
		return fileInfo{name: name, size: int64(len(file.data)), when: file.when}, nil
	}
	files, err := d.stored()
	if err != nil {
		return fileInfo{}, err
	}
	if e, found := files[name]; found {
		return fileInfo{name: name, size: e.Size, when: e.Uploaded, id: e.ID}, nil
	}
	if d.folders[name] {
		return folder, nil
	}
	for other := range files {
		if strings.HasPrefix(other, name+"/") {
			return folder, nil
		}
	}
	for other := range d.folders {
		if strings.HasPrefix(other, name+"/") {
			return folder, nil
		}
	}
	return fileInfo{}, fs.ErrNotExist
}

// list returns what is directly inside a folder.
func (d *drive) list(folder string) ([]os.FileInfo, error) {
	files, err := d.stored()
	if err != nil {
		return nil, err
	}
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}

	found := map[string]fileInfo{}
	add := func(name string, info fileInfo) {
		rest, inside := strings.CutPrefix(name, prefix)
		if !inside || rest == "" {
			return
		}
		// A name that goes on after a slash shows the folder it is in.
		child, _, deeper := strings.Cut(rest, "/")
		if deeper {
			info = fileInfo{when: d.since, dir: true}
		}
		info.name = prefix + child
		found[child] = info
	}
	for name := range d.folders {
		add(name, fileInfo{when: d.since, dir: true})
	}
	for name, e := range files {
		add(name, fileInfo{size: e.Size, when: e.Uploaded, id: e.ID})
	}
	for name, file := range d.local {
		add(name, fileInfo{size: int64(len(file.data)), when: file.when})
	}

	infos := make([]os.FileInfo, 0, len(found))
	for _, info := range found {
		infos = append(infos, info)
	}
	return infos, nil
}

func (d *drive) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	info, err := d.find(clean(name))
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (d *drive) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	name = clean(name)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.find(name); err == nil {
		return fs.ErrExist
	}
	if in, err := d.find(parent(name)); err != nil || !in.dir {
		return fs.ErrNotExist
	}
	d.folders[name] = true
	return nil
}

func (d *drive) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	name = clean(name)
	d.mu.Lock()
	defer d.mu.Unlock()

	if flag&(os.O_CREATE|os.O_TRUNC) != 0 {
		if info, err := d.find(name); err == nil && info.dir {
			return nil, fs.ErrExist
		}
		if in, err := d.find(parent(name)); err != nil || !in.dir {
			return nil, fs.ErrNotExist
		}
		size, _ := ctx.Value(sizeKey{}).(int64)
		return &writing{d: d, ctx: ctx, name: name, when: time.Now(), expect: size}, nil
	}

	info, err := d.find(name)
	if err != nil {
		return nil, err
	}
	var content io.ReadSeeker = bytes.NewReader(d.local[name].data)
	if info.id != "" {
		content = d.store.Open(ctx, d.key, info.id, info.size)
	}
	return &reading{ReadSeeker: content, d: d, ctx: ctx, info: info}, nil
}

func (d *drive) RemoveAll(ctx context.Context, name string) error {
	name = clean(name)
	if name == "" {
		return fs.ErrPermission
	}
	d.mu.Lock()
	gone, err := d.remove(name)
	d.mu.Unlock()

	for _, e := range gone {
		d.discard(e)
		d.note("deleted " + e.Name)
	}
	return err
}

// remove takes a name off the drive, with everything inside it when it is a
// folder. It returns the stored files that went, for discard.
func (d *drive) remove(name string) ([]entry, error) {
	if _, err := d.find(name); err != nil {
		return nil, err
	}
	// A folder that lost its last file stays until the drive is ejected.
	if in := parent(name); in != "" {
		d.folders[in] = true
	}
	if _, found := d.local[name]; found {
		delete(d.local, name)
		return nil, nil
	}

	files, err := d.stored()
	if err != nil {
		return nil, err
	}
	var gone []entry
	ids := map[string]bool{}
	for other, e := range files {
		if under(other, name) {
			gone = append(gone, e)
			ids[e.ID] = true
		}
	}
	if len(gone) > 0 {
		err := d.update(func(entries []entry) []entry {
			var left []entry
			for _, e := range entries {
				if !ids[e.ID] {
					left = append(left, e)
				}
			}
			return left
		})
		if err != nil {
			return nil, err
		}
	}
	maps.DeleteFunc(d.local, func(other string, _ kept) bool { return under(other, name) })
	maps.DeleteFunc(d.folders, func(other string, _ bool) bool { return under(other, name) })
	return gone, nil
}

// discard deletes a file's messages from the chat. Telegram refuses that for
// messages older than 48 hours. The file is off the drive either way.
func (d *drive) discard(e entry) {
	if err := d.store.Delete(context.Background(), d.key, e.ID); err != nil {
		d.note(fmt.Sprintf("the messages of %s are still in the chat: %v", e.Name, err))
	}
}

// Rename moves a file, or a folder with everything inside it. Nothing is
// uploaded again: only the names in the index change.
func (d *drive) Rename(ctx context.Context, from, to string) error {
	from, to = clean(from), clean(to)
	if from == "" || to == "" || under(to, from) {
		return fs.ErrPermission
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.find(from); err != nil {
		return err
	}
	if _, err := d.find(to); err == nil {
		return fs.ErrExist
	}
	if in, err := d.find(parent(to)); err != nil || !in.dir {
		return fs.ErrNotExist
	}
	moved := func(name string) string {
		if under(name, from) {
			return to + strings.TrimPrefix(name, from)
		}
		return name
	}

	files, err := d.stored()
	if err != nil {
		return err
	}
	names := map[string]string{}
	for name, e := range files {
		if under(name, from) {
			names[e.ID] = moved(name)
		}
	}
	if len(names) > 0 {
		err := d.update(func(entries []entry) []entry {
			for i, e := range entries {
				if name, found := names[e.ID]; found {
					entries[i].Name = name
				}
			}
			return entries
		})
		if err != nil {
			return err
		}
	}

	local, folders := map[string]kept{}, map[string]bool{}
	for name, file := range d.local {
		local[moved(name)] = file
	}
	for name := range d.folders {
		folders[moved(name)] = true
	}
	if in := parent(from); in != "" {
		folders[in] = true
	}
	d.local, d.folders = local, folders
	return nil
}

// put writes a file that has just been uploaded into the index. A file that
// had the name before makes way for it, and its messages are deleted.
func (d *drive) put(added entry) error {
	d.mu.Lock()
	files, err := d.stored()
	old, replaced := files[added.Name]
	if err == nil {
		err = d.update(func(entries []entry) []entry {
			var left []entry
			for _, e := range entries {
				if !replaced || e.ID != old.ID {
					left = append(left, e)
				}
			}
			return append(left, added)
		})
	}
	if err == nil {
		delete(d.local, added.Name)
	}
	d.mu.Unlock()

	if err != nil {
		return fmt.Errorf("uploaded as %s, but the index could not be saved: %w", added.ID, err)
	}
	d.note("saved " + added.Name + "  " + formatSize(added.Size))
	if replaced {
		d.discard(old)
	}
	return nil
}

// failed is called when a file could not be saved. Finder does not tell: it
// shows the copy as done whatever the drive answers. So the empty file Finder
// made first is taken away again, and the alert has to say what happened.
func (d *drive) failed(name string, err error) {
	d.mu.Lock()
	delete(d.local, name)
	delete(d.local, path.Join(parent(name), "._"+path.Base(name)))
	d.mu.Unlock()
	if errors.Is(err, context.Canceled) {
		err = errors.New("the copy was cut off")
	}
	d.alert(fmt.Sprintf("%s was not saved: %v", name, err))
}

// reading is a file or a folder that was opened to look at it. Opening asks
// Telegram for nothing, and a file's first Read does: listing a folder opens
// every file in it only to see its size and time.
type reading struct {
	io.ReadSeeker
	d    *drive
	ctx  context.Context
	info fileInfo
}

// Read does not return when a download fails halfway. The download would end
// there, and Finder takes a download that ends early for the whole file. One
// that stops moving it gives up on after a minute or so, with an error.
func (r *reading) Read(p []byte) (int, error) {
	n, err := r.ReadSeeker.Read(p)
	if err != nil && err != io.EOF {
		if r.ctx.Err() == nil {
			r.d.note(fmt.Sprintf("%s could not be read to the end: %v", r.info.name, err))
		}
		<-r.ctx.Done()
	}
	return n, err
}

func (r *reading) Close() error               { return nil }
func (r *reading) Write([]byte) (int, error)  { return 0, fs.ErrPermission }
func (r *reading) Stat() (os.FileInfo, error) { return r.info, nil }
func (r *reading) Readdir(int) ([]os.FileInfo, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	return r.d.list(r.info.name)
}

// writing is a file that is being saved. Its bytes go on to Telegram while
// they come in, so a big file is never held in memory or on disk as a whole.
// A helper file of Finder's, or a file that gets no bytes at all, is only
// remembered in the drive's local files.
type writing struct {
	d    *drive
	ctx  context.Context
	name string
	when time.Time
	size int64
	// expect is how big Finder said the file is, if it did.
	expect int64

	kept []byte
	// pipe leads to the upload, which starts with the first bytes. done gets
	// the upload's error when it is over, and id is set just before that.
	pipe *io.PipeWriter
	done chan error
	id   string
}

func (w *writing) Write(p []byte) (int, error) {
	if helper(w.name) {
		w.kept = append(w.kept, p...)
		return len(p), nil
	}
	if len(p) == 0 {
		return 0, nil
	}
	if w.pipe == nil {
		reader, writer := io.Pipe()
		w.pipe, w.done = writer, make(chan error, 1)
		w.d.progress.begin(w, w.name, w.expect)
		go func() {
			id, err := w.d.store.Upload(w.ctx, w.d.key, reader)
			// A Write that waits for the upload to take its bytes gets the error.
			reader.CloseWithError(err)
			w.id = id
			w.done <- err
		}()
	}
	n, err := w.pipe.Write(p)
	w.size += int64(n)
	return n, err
}

// broken is why the copy stopped: the body could not be read, or the whole
// request was cut off. The body's own error is remembered because the request
// is not always cancelled before the file is closed.
func (w *writing) broken() error {
	if body, ok := w.ctx.Value(bodyKey{}).(*observedBody); ok && body.err != nil {
		return body.err
	}
	return w.ctx.Err()
}

func (w *writing) Close() error {
	if w.pipe == nil {
		if !helper(w.name) {
			if err := w.broken(); err != nil {
				w.d.failed(w.name, err)
				return err
			}
		}
		w.d.mu.Lock()
		w.d.local[w.name] = kept{w.kept, w.when}
		w.d.mu.Unlock()
		return nil
	}
	// If Finder broke off, the upload fails here, and half a file is never
	// taken for a whole one.
	w.pipe.CloseWithError(w.broken())
	err := <-w.done
	w.d.progress.end(w)
	if err == nil {
		err = w.d.put(entry{Name: w.name, ID: w.id, Size: w.size, Uploaded: w.when})
	}
	if err != nil {
		w.d.failed(w.name, err)
	}
	return err
}

func (w *writing) Read([]byte) (int, error)           { return 0, fs.ErrPermission }
func (w *writing) Seek(int64, int) (int64, error)     { return 0, fs.ErrPermission }
func (w *writing) Readdir(int) ([]os.FileInfo, error) { return nil, fs.ErrPermission }
func (w *writing) Stat() (os.FileInfo, error) {
	return fileInfo{name: w.name, size: w.size + int64(len(w.kept)), when: w.when}, nil
}

// fileInfo is what Finder gets to know about a file or a folder.
type fileInfo struct {
	name string
	size int64
	when time.Time
	dir  bool
	id   string // of a stored file
}

func (f fileInfo) Name() string       { return path.Base(f.name) }
func (f fileInfo) Size() int64        { return f.size }
func (f fileInfo) ModTime() time.Time { return f.when }
func (f fileInfo) IsDir() bool        { return f.dir }
func (f fileInfo) Sys() any           { return nil }

func (f fileInfo) Mode() os.FileMode {
	if f.dir {
		return os.ModeDir | 0o755
	}
	return 0o644
}

// ContentType goes by the name. Without it the webdav package reads the
// start of every file in a listing to guess, a download from Telegram each.
func (f fileInfo) ContentType(ctx context.Context) (string, error) {
	if kind := mime.TypeByExtension(path.Ext(f.name)); kind != "" {
		return kind, nil
	}
	return "application/octet-stream", nil
}

// locks hands out the locks Finder asks for and never refuses a write because
// of one. Finder will only write to a drive that can lock, but only one
// program uses this drive. With the webdav package's own locks, a lock that
// Finder never gave back, after a copy that was cut off, kept that name from
// being copied or deleted until the drive was ejected.
type locks struct {
	mu      sync.Mutex
	count   int
	granted map[string]webdav.LockDetails
}

func newLocks() *locks { return &locks{granted: map[string]webdav.LockDetails{}} }

func (l *locks) Confirm(time.Time, string, string, ...webdav.Condition) (func(), error) {
	return func() {}, nil
}

func (l *locks) Create(_ time.Time, details webdav.LockDetails) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count++
	token := fmt.Sprintf("opaquelocktoken:taasant-%d", l.count)
	l.granted[token] = details
	return token, nil
}

func (l *locks) Refresh(_ time.Time, token string, duration time.Duration) (webdav.LockDetails, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	details, ok := l.granted[token]
	if !ok {
		return webdav.LockDetails{}, webdav.ErrNoSuchLock
	}
	details.Duration = duration
	l.granted[token] = details
	return details, nil
}

func (l *locks) Unlock(_ time.Time, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.granted[token]; !ok {
		return webdav.ErrNoSuchLock
	}
	delete(l.granted, token)
	return nil
}
