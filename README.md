<p align="center">
  <img src="site/logo.svg" width="170" alt="The taasant logo: a purple pixel ant">
</p>

# taasant

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

The code is small. The part that stores files uses nothing outside the standard library, and the menu is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). It works, but treat it as a hobby project and not as your only backup.

Please be reasonable about what you store. This runs on Telegram's goodwill.

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

This takes about five minutes, and you only do it once. You need three things: a bot, your chat ID, and a key.

**1. Make a bot.** In Telegram, open a chat with [@BotFather](https://t.me/BotFather), send `/newbot`, and answer its two questions. It hands you a token that looks like `123456:ABC...`.

**2. Say hello to your bot.** Open the bot you just made and press **Start**. This matters: a bot is not allowed to message you until you have messaged it.

**3. Find your chat ID.** Open [@userinfobot](https://t.me/userinfobot) and press Start. It replies with a number. That number is your chat ID.

**4. Make a key.**

```bash
taasant keygen
```

**5. Save all three.** Add these lines to the end of `~/.zshrc` (or `~/.bashrc` if you use bash), with your own values:

```bash
export TG_BOT_TOKEN="123456:ABC..."
export TG_CHAT_ID="1234567890"
export TAAS_KEY="the long string from keygen"
```

Open a new terminal window, or run `source ~/.zshrc`, and you're done.

One warning before you move on. **The key is the only thing that can read your files.** Run `keygen` once, put a copy in your password manager, and never change it. If you lose it, your files are gone for good. Nobody can recover them, including me.

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
taasant list                       # see everything, newest first
taasant list rep                   # search
taasant download report            # get it back
taasant download report copy.pdf   # get it back under another name
taasant delete report              # remove it
taasant update                     # get the newest version of taasant
```

Uploads and downloads show a progress bar that moves as the bytes go out. If the connection drops, taasant tries again a few times before it gives up.

### In scripts

When the output is not a terminal, taasant drops the colours and the progress bar. `upload` then prints only the file's ID, so you can capture it:

```bash
id=$(taasant upload backup.tar)
```

Set `NO_COLOR=1` if you want plain text in a terminal too.

### You don't have to type the whole name

taasant is forgiving about names. `taasant download hol` finds `holiday.jpg`, and so does `taasant download hjp`. It looks for an exact name first, then for names containing what you typed, then for names with those letters in that order.

If more than one file matches, it shows you the matches and does nothing. It will never guess which file you meant to delete.

### It won't overwrite your files

If a file with that name already exists where you are, `download` stops with `file exists`. Give it a different name, as in the `copy.pdf` example above.

## What ends up where

**In your Telegram chat:** your files, as messages with an attachment called `part.bin`. A small file is two messages (the file and a tiny list of its parts). A big one is several. They are encrypted, so opening one from the Telegram app gets you nothing useful. Always use `taasant download`.

**On your computer:** a list of what you have uploaded, called the index. It lives at `~/Library/Application Support/taas/index.json` on a Mac and `~/.config/taas/index.json` on Linux. Set `TAAS_INDEX` if you want it somewhere else.

**A backup of that list:** every time you upload or delete, taasant also sends an encrypted copy of the index to your chat, and notes where it is in a file called `index.json.remote` next to the index. Each backup adds a couple of messages to the chat, so expect some clutter.

## Things worth knowing

- **Lose the key, lose the files.** Yes, this is here twice.
- **Look after the index.** If you lose it, your files are still in the chat, but taasant no longer knows which messages are which. Copying the `taas` folder somewhere safe now and then is enough.
- **You can only delete recent files.** Telegram lets a bot delete messages for 48 hours after sending them. After that, `taasant delete` will fail, and you would have to remove the messages by hand in the app.
- **Big files need memory.** Files are handled in 19 MiB parts and several are in memory at once. A very large download can use a lot, because all its parts are fetched together.
- **One computer.** The index is not synced. If you use taasant on two machines, each has its own list.

## When something goes wrong

**`the bot can't send messages to the bot`**
Your `TG_CHAT_ID` is a bot's ID. This usually means you used the number at the start of the token. Use the number @userinfobot gave you.

**`bot can't initiate conversation with a user`**
You skipped step 2. Open your bot in Telegram and press Start.

**`401 Unauthorized`**
The token is wrong or has been revoked. Get a fresh one from @BotFather.

**`cipher: message authentication failed`**
`TAAS_KEY` is not the key this file was uploaded with.

**`TAAS_KEY must be 64 hex characters`**
The key is missing or got cut off when you pasted it. Check `echo $TAAS_KEY`.

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

`cmd/taasant/` is the command line and the index. `tgstore/` does the splitting, the encryption and the talking to Telegram.

## Credits

The storage design was inspired by [golang-design/tgstore](https://github.com/golang-design/tgstore).

## License

[MIT](LICENSE). Use it, change it and share it freely; keep the copyright notice.
