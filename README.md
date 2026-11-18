<p align="center">
  <img src="site/logo.svg" width="170" alt="The taasant logo: a purple pixel ant">
</p>

# taasant

[![Downloads](https://img.shields.io/github/downloads/vedantlavale/taasant/total?label=downloads&color=6d5df6)](https://github.com/vedantlavale/taasant/releases)
[![Latest release](https://img.shields.io/github/v/release/vedantlavale/taasant?color=6d5df6)](https://github.com/vedantlavale/taasant/releases/latest)
[![Stars](https://img.shields.io/github/stars/vedantlavale/taasant?color=e3a008)](https://github.com/vedantlavale/taasant)

Telegram lets a bot store files in a chat for free. taasant turns that into a little encrypted drive you use from the terminal.

Docs: [taasant.vedantworks.com](https://taasant.vedantworks.com)

![The taasant menu: Upload, Files, Download and Delete](assets/menu-home.png)

Type `taasant` in a folder to get this menu, or use plain commands:

```console
$ taasant upload holiday.jpg
✓ holiday.jpg  45.0 MiB  uploaded in 12.4s

$ taasant list
  NAME               SIZE   UPLOADED
  holiday.jpg    45.0 MiB   today 12:09
  report.pdf      1.2 MiB   yesterday
  notes.txt       4.0 KiB   3 Oct

  3 files, 46.2 MiB

$ taasant download hol
✓ saved holiday.jpg
```

Everything is encrypted on your computer before it is sent, so Telegram only ever sees scrambled bytes. Big files are split up for you, and you get them back by name, not by some long ID.

The code is small. The part that stores files uses nothing outside the standard library, the menu is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), and the drive in Finder with Go's [webdav](https://pkg.go.dev/golang.org/x/net/webdav) package. It works, but treat it as a hobby project and not as your only backup.

Under its [bot terms](https://telegram.org/tos/bot-developers), Telegram might restrict a bot that is used for storage. To be safe, make the bot from a second Telegram account, and be reasonable about what you store.

## New in version 0.2

- **A menu.** Run `taasant` with nothing after it to browse folders, mark one file or several, and upload, download or delete without typing names.
- **`taasant update`** fetches the newest release and replaces the program in place.
- **Uploads survive a shaky connection.** A broken connection is tried again a few times before taasant gives up.
- **The progress bar moves as bytes go out**, not only when a whole part is done.

## Install

### The quick way (macOS and Linux)

One command downloads the right file and puts it in `/usr/local/bin`. No Go needed.

```bash
curl -fsSL https://taasant.vedantworks.com/install.sh | sh
```

Or do the same by hand. Pick the line for your machine:

```bash
# Apple Silicon Mac
curl -L -o taasant https://github.com/vedantlavale/taasant/releases/latest/download/taasant-macos-arm64

# Intel Mac
curl -L -o taasant https://github.com/vedantlavale/taasant/releases/latest/download/taasant-macos-intel

# Linux
curl -L -o taasant https://github.com/vedantlavale/taasant/releases/latest/download/taasant-linux-amd64
```

Then make it runnable and move it somewhere on your `PATH`:

```bash
chmod +x taasant
sudo mv taasant /usr/local/bin/
taasant keygen
```

If that last command prints a long string of letters and numbers, you're set.

### Updating

From version 0.2 on, run `taasant update`. Version 0.1 does not have that command, so install 0.2 once the same way as above.

On Windows, download `taasant-windows.exe` from the [releases page](https://github.com/vedantlavale/taasant/releases/latest) and run it from a terminal.

### "Apple could not verify taasant is free of malware"

You will see this on a Mac if you download the file with your browser instead of `curl`. Nothing is wrong with the file. Apple shows this for any program whose author hasn't paid for a developer certificate, and I haven't.

Click **Done** (not "Move to Bin"), then tell macOS you trust it:

```bash
xattr -d com.apple.quarantine ~/Downloads/taasant-macos-arm64
```

After that, carry on with the `chmod` and `mv` steps above. You can also go to System Settings → Privacy & Security and press **Open Anyway**.

Downloading with `curl`, as shown above, skips the warning entirely. Also, don't double-click the file: taasant is a terminal program and has no window.

### If you have Go

```bash
go install github.com/vedantlavale/taasant/cmd/taasant@latest
```

Or build it yourself:

```bash
git clone https://github.com/vedantlavale/taasant.git
cd taasant
go build -o taasant ./cmd/taasant
```

You need Go 1.27 or newer.

## Set it up

This takes about five minutes, and you only do it once. Run `taasant` and it asks for what it needs.

**1. Run `taasant`.** The first time, a setup screen opens. The menu comes after it.

**2. Make a bot and paste its token.** Use a spare Telegram account for this if you can. In Telegram, open a chat with [@BotFather](https://t.me/BotFather), send `/newbot`, and answer its two questions. It hands you a token that looks like `123456:ABC...`. Paste it into taasant, which checks it with Telegram.

**3. Choose where the files go.** taasant asks where the bot should keep them:

- **A private channel (recommended).** Your files get a place of their own, away from the bot's chat. In Telegram, create a new private channel, add your bot to it as an administrator (leave its rights switched on, the bot has to post there), and post any message in the channel.
- **The chat with the bot.** Simpler, with nothing else to create. Open your bot in Telegram and press **Start**.

Either way, taasant waits until it hears from that place, shows its name and asks if it is the right one. There is no chat ID to look up. If it found the wrong one, press Esc to look again.

**4. Done.** taasant makes your encryption key, saves everything, and opens the menu.

One warning before you move on. **The key is the only thing that can read your files.** taasant keeps it in its settings file: `~/Library/Application Support/taas/config.json` on a Mac, `~/.config/taas/config.json` on Linux. Put a copy of that file in your password manager. If you lose it, your files are gone for good. Nobody can recover them, including me.

The setup screen is new in version 0.3. On an older version, run `taasant update` first.

### Setting it up by hand

Three environment variables do the same job, and each one that is set wins over the settings file. That is useful in scripts, and a setup made this way before version 0.3 keeps working.

```bash
export TG_BOT_TOKEN="123456:ABC..."
export TG_CHAT_ID="-1001234567890"
export TAAS_KEY="64 hex characters, taasant keygen prints a new one"
```

`TG_CHAT_ID` is the number of the place the files go. For a private channel it is negative and starts with `-100`, as above. For the chat with the bot it is a positive number, your own user ID. The Telegram app does not show either one. The easy way is to let the setup screen find it: run `taasant` once with `TAAS_CONFIG` set to a throwaway path, then read `chat_id` from that file.

### Other places for your files

Telegram is the free choice. If you would rather pay for storage, taasant also works with **Cloudflare R2**, **Amazon S3** and **MinIO**, and with any other service that speaks the S3 API. The first question in the setup screen asks which one you want, and `taasant setup` adds another later. You can keep several at once: one is the default, and each file remembers where it went, so downloads and deletes find it by themselves.

```bash
taasant upload report.pdf            # goes to your default place
taasant upload --to r2 report.pdf    # goes to the place called "r2"
taasant list                         # shows a WHERE column once you use more than one
```

The setup asks for what each service needs, then tries your bucket with a small test file, so a wrong key shows up right away. The files are encrypted on your computer, as with Telegram, and are stored under random names. A bucket holds files up to 156 GiB each.

For a bucket there are no environment variables, only the settings file. This is what `taasant` writes for R2, and you can write the same by hand. `default` is where new files go, and `backends` can hold several places.

```json
{
  "key": "64 hex characters",
  "default": "r2",
  "backends": {
    "r2": {
      "type": "s3",
      "endpoint": "https://<account id>.r2.cloudflarestorage.com",
      "region": "auto",
      "bucket": "my-bucket",
      "access_key": "...",
      "secret_key": "..."
    }
  }
}
```

## Using it

There are two ways: a menu, and plain commands. Both do the same things.

### The menu

This is the simplest way. Go to the folder your files are in and type `taasant` with nothing after it. Move with the arrow keys and press Enter. The menu stays open until you press Q, and Esc goes back to the start.

**Upload** shows the folder you are in. `→` opens a folder and `←` goes up. Space marks a file, and the marks stay as you move between folders. Enter uploads everything you marked, or the file under the cursor if you marked nothing.

![The upload screen with two files marked](assets/menu-browse.png)

While files are moving there is a line for each one. Esc cancels the ones that have not started.

![Three uploads: one done, one in progress, one waiting](assets/menu-work.png)

**Files** shows what you have stored from this computer. A bot cannot ask Telegram what is in the chat, so the list is the one taasant keeps here. **Download** and **Delete** show the same list, and Enter picks the file. A download lands in the folder you started taasant from. Delete asks first.

![The list of stored files](assets/menu-files.png)

### Commands

```bash
taasant upload report.pdf          # store a file
taasant setup                      # add a place for your files
taasant list                       # see everything, newest first
taasant list rep                   # search
taasant download report            # get it back
taasant download report copy.pdf   # get it back under another name
taasant delete report              # remove it
taasant mount                      # show the files as a drive in Finder (Mac)
taasant autostart                  # show the drive at every login (Mac)
taasant update                     # get the newest version of taasant
```

Uploads and downloads show a progress bar that moves as the bytes go out. If the connection drops, taasant tries again a few times before it gives up.

### A drive in Finder

On a Mac, `taasant mount` puts your files in Finder's sidebar, under **Locations**:

```console
$ taasant mount
✓ the drive is in Finder, under Locations. Ctrl-C ejects it
  saved holiday.jpg  45.0 MiB
```

The drive is called `taasant.localhost`. Drag files in to store them, drag them out or open them to get them back, and make folders, rename and delete as on any disk. It stays for as long as the command runs. Press Ctrl-C in the terminal, or eject the drive in Finder, to take it away.

To have the drive without a terminal, run `taasant autostart` once. It is in Finder at once, and again at every login, with no window opened and nothing left running in a terminal. Eject it in Finder and it stays away until the next login. `taasant autostart off` stops it for good. Problems are written to `~/Library/Logs/taasant.log`. If you move or update the program, run `taasant autostart` again so the job points at it.

No app is needed and nothing else has to be installed. taasant serves the files to Finder from your own computer, at an address other computers cannot reach.

A few things work differently from a normal disk:

- **A copy takes as long as the upload.** Finder is done with a file when the file is in Telegram, not before. Finder's own progress runs ahead of that, so watch the percent in the Mac app's menu bar icon, or its window, for the real one.
- **Finder does not tell you when a copy to the drive failed.** taasant does, with a notification and a line in the terminal. The file is then not on the drive, so copy it again.
- **Opening a file downloads it.** A big file starts to arrive after its first 19 MiB.
- **Icon previews are off.** Finder draws them by reading the files, which would download every file it shows. taasant switches them off in the drive's window, and macOS may ask once whether your terminal may control Finder for that. A folder inside the drive has its own setting, under View → Show View Options.
- **If a download breaks off halfway**, Finder waits for a minute or so and then reports an error.
- **A folder is part of a file's name.** `taasant list` shows `photos/a.jpg`. Folders and files that are empty are not stored, and are forgotten when the drive is ejected.
- **Deleting a file older than 48 hours** takes it off the drive, but Telegram keeps its messages. They are encrypted, and the terminal names the file.
- **Two files with one name.** `taasant upload` lets you store the same name twice. The drive shows the newer one. Delete or rename it there and the older one shows up.

### The Mac app

If you would rather click than type, there is a small Mac app: an icon in the menu bar and a window with your files. Download `Taasant.zip` from the [releases page](https://github.com/vedantlavale/taasant/releases/latest), unzip it and drag `Taasant.app` to Applications.

- **The menu bar icon** shows whether the drive is in Finder, and connects or ejects it. The panel uses the macOS glass effect; the window stays dark with the site's ant. **Start at login** brings the drive back every time you log in.
- **The window** (menu → Show files…) has four sections: Files, Transfers, Drive and Settings. Drop files on it to upload them. A delete asks for confirmation in a dialog, and files can be filtered by kind and sorted by date, name or size. The menu bar popover has the same sections in a compact form, with the file list, its filters, and a bar that follows an upload or a delete. While you copy files to the drive in Finder, the menu bar icon shows the percent that has reached Telegram. Open the app again from Finder or Spotlight if you cannot see the icon.
- **The first time**, the window offers to set taasant up. That opens a terminal and asks the same questions as `taasant` does, because the bot token and the key are best pasted in there.

The app has the `taasant` program inside it and does everything through it, so the app and the terminal always see the same files. It has no developer certificate either, so the first time macOS says it cannot verify it. Press **Done**, then go to System Settings → Privacy & Security and press **Open Anyway**. Or run `xattr -dr com.apple.quarantine /Applications/Taasant.app`.

To build it yourself, run `macos/build.sh`. It needs Go and Swift (the command line tools are enough) and leaves `macos/dist/Taasant.app`.

### In scripts

When the output is not a terminal, taasant drops the colours and the progress bar. `upload` then prints only the file's ID, so you can capture it:

```bash
id=$(taasant upload backup.tar)
```

Set `NO_COLOR=1` if you want plain text in a terminal too.

Add `--json` to `list`, `upload`, `download` or `delete` for output a program can read. `taasant list --json` prints all files as one array, and an upload, download or delete prints `{"done":..,"total":..}` as it goes. This is what the Mac app uses.

### You don't have to type the whole name

taasant is forgiving about names. `taasant download hol` finds `holiday.jpg`, and so does `taasant download hjp`. It looks for an exact name first, then for names containing what you typed, then for names with those letters in that order.

If more than one file matches, it shows you the matches and does nothing. It will never guess which file you meant to delete.

### It won't overwrite your files

If a file with that name already exists where you are, `download` stops with `file exists`. Give it a different name, as in the `copy.pdf` example above.

## What ends up where

**In your Telegram channel or chat:** your files, as messages with an attachment called `part.bin`. A small file is two messages (the file and a tiny list of its parts). A big one is several. They are encrypted, so opening one from the Telegram app gets you nothing useful. Always use `taasant download`.

**On your computer:** a list of what you have uploaded, called the index. It lives at `~/Library/Application Support/taas/index.json` on a Mac and `~/.config/taas/index.json` on Linux. Set `TAAS_INDEX` if you want it somewhere else.

**A backup of that list:** every time you upload or delete, taasant also sends an encrypted copy of the index to your default place, and notes where it is in a file called `index.json.remote` next to the index: the place, then the ID. Each backup adds a couple of messages to a Telegram channel or chat, so expect some clutter. To get it back on another computer, run `taasant download --to <place> <id> index.json`.

## Things worth knowing

- **Lose the key, lose the files.** Yes, this is here twice.
- **Look after the index.** If you lose it, your files are still in the channel or chat, but taasant no longer knows which messages are which. Copying the `taas` folder somewhere safe now and then is enough.
- **Old files leave messages behind.** Telegram lets a bot delete messages for 48 hours after sending them. Deleting an older file still takes it off the drive, but its encrypted messages stay in the chat. taasant names the file when that happens.
- **Big files need memory.** Files are handled in 19 MiB parts and several are in memory at once. A very large download can use a lot, because all its parts are fetched together.
- **Use a second Telegram account for the bot.** Telegram might restrict a bot that is used for storage, so keep your main account out of it.
- **One computer.** The index is not synced. If you use taasant on two machines, each has its own list.

## When something goes wrong

**`there is no bot token, run "taasant" in a terminal to set up`**
taasant has not been set up on this computer. Run `taasant` with nothing after it and answer its questions.

**`the bot can't send messages to the bot`**
Your `TG_CHAT_ID` is a bot's ID. This usually means you used the number at the start of the token. Use the number @userinfobot gave you, or let the setup screen find the place for you.

**Setup keeps waiting after you added the bot to a channel**
taasant listens for the bot being added to a channel and for new posts there, and it hears nothing from before it started. Make sure the bot is an administrator of that channel, with its rights on, and post a new message in the channel. If you chose the chat with the bot instead, press **Start** in it.

**`chat not found`, or `bot is not a member of the channel chat`**
The `TG_CHAT_ID` is wrong, or the bot is no longer in the channel. For a channel the number starts with `-100`. Add the bot to the channel as an administrator again, or run setup again.

**`bot can't initiate conversation with a user`**
The bot may not write to you until you have written to it. Open your bot in Telegram and press Start.

**`401 Unauthorized`**
The token is wrong or has been revoked. Get a fresh one from @BotFather.

**`cipher: message authentication failed`**
The key in use is not the key this file was uploaded with. Check whether `TAAS_KEY` is set and differs from the key in the settings file.

**`the key must be 64 hex characters`**
The key is missing or got cut off when it was pasted. If you set `TAAS_KEY` yourself, check `echo $TAAS_KEY`.

**`file exists`**
`download` won't overwrite. Give it another output name.

## How it works, briefly

A bot can upload files up to 50 MB but can only download ones up to 20 MB, so taasant cuts every file into 19 MiB parts. Each part is encrypted with AES-256-GCM and sent as its own message. Then it sends one more small encrypted file, the manifest, which lists the parts in order. The manifest's ID is what gets saved in your index next to the file name.

Downloading does the reverse: look up the name, fetch the manifest, fetch the parts, decrypt them, and write them out in order.

## Working on the code

```bash
go test ./...          # no token or internet needed, it uses a fake Telegram
go test -race ./...    # also checks for concurrency mistakes
```

`cmd/taasant/` is the command line, the index and the drive. `tgstore/` does the splitting, the encryption and the talking to Telegram. `macos/` is the Mac app, written in Swift.

## Credits

The storage design was inspired by [golang-design/tgstore](https://github.com/golang-design/tgstore).

## License

[MIT](LICENSE). Use it, change it and share it freely; keep the copyright notice.
