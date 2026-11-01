package s3store

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

// TestLive runs against a real bucket, R2, S3, MinIO or any other, and is
// skipped unless it is told which. It leaves nothing behind.
//
//	TAAS_S3_TEST_ENDPOINT=https://<account>.r2.cloudflarestorage.com \
//	TAAS_S3_TEST_REGION=auto TAAS_S3_TEST_BUCKET=... \
//	TAAS_S3_TEST_ACCESS_KEY=... TAAS_S3_TEST_SECRET_KEY=... go test ./s3store -run Live
func TestLive(t *testing.T) {
	env := func(name string) string { return os.Getenv("TAAS_S3_TEST_" + name) }
	if env("ENDPOINT") == "" {
		t.Skip("TAAS_S3_TEST_ENDPOINT is not set")
	}
	store := New(env("ENDPOINT"), env("REGION"), env("BUCKET"), env("ACCESS_KEY"), env("SECRET_KEY"))
	ctx := context.Background()
	if err := store.Check(ctx); err != nil {
		t.Fatal(err)
	}

	// Three parts of the real size: more than one part, and all parts but the last equal.
	key, content := randomBytes(32), randomBytes(2*chunkSize+12345)
	id, err := store.Upload(ctx, key, bytes.NewReader(content), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Delete(ctx, key, id)

	var got bytes.Buffer
	if err := store.Download(ctx, key, id, &got, nil); err != nil || !bytes.Equal(got.Bytes(), content) {
		t.Fatalf("download: other bytes, or %v", err)
	}
	r := store.Open(ctx, key, id, int64(len(content)))
	r.Seek(chunkSize+7, io.SeekStart)
	part := make([]byte, 100)
	if _, err := io.ReadFull(r, part); err != nil || !bytes.Equal(part, content[chunkSize+7:chunkSize+107]) {
		t.Fatalf("a ranged read: other bytes, or %v", err)
	}
	if err := store.Delete(ctx, key, id); err != nil {
		t.Fatal(err)
	}
	if err := store.Download(ctx, key, id, io.Discard, nil); err == nil {
		t.Error("the object can still be downloaded after a delete")
	}
}
