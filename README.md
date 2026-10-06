# taasant

`taasant` is a small command-line tool that uses a Telegram chat as encrypted file storage.

Your files are encrypted before they leave your computer. Telegram stores only encrypted data, while `taasant` keeps a local index so you can find files by name.

```console
$ taasant upload holiday.jpg
104:BQACAgUAAxkDAAMG...

$ taasant list
2026-10-05 12:09    45.0 MiB  holiday.jpg
    104:BQACAgUAAxkDAAMG...

$ taasant download hol
saved holiday.jpg
```

> Use a private chat and store only files you are allowed to store. Please do not abuse Telegram's service.

## How it works

Large files are split into parts smaller than Telegram's bot download limit. Each part is encrypted with AES-256-GCM and uploaded as a Telegram document. A small encrypted manifest records the parts that make up the original file.

Uploads use several workers so multiple parts can be sent at the same time. Downloads fetch parts concurrently, then write them to disk in their original order.

The local index records each file's name, size, upload time, and manifest ID. After an upload or delete, the current index is also encrypted and uploaded to Telegram as a backup.

## Installation

### Download a binary

No Go needed. Pick the file for your system from the [latest release](https://github.com/vedantlavale/taasant/releases/latest):

| System | File |
| --- | --- |
| macOS, Apple Silicon | `taasant-macos-arm64` |
| macOS, Intel | `taasant-macos-intel` |
| Linux, x86-64 | `taasant-linux-amd64` |
| Windows, x86-64 | `taasant-windows.exe` |

On macOS or Linux, make it runnable and put it on your `PATH`. For example, on an Apple Silicon Mac:

```bash
curl -L -o taasant https://github.com/vedantlavale/taasant/releases/latest/download/taasant-macos-arm64
chmod +x taasant
sudo mv taasant /usr/local/bin/
```

The binaries are not signed. If macOS refuses to open the file, clear the download flag once:

```bash
xattr -d com.apple.quarantine /usr/local/bin/taasant
```

### Install with Go

If you have [Go 1.27 or newer](https://go.dev/dl/):

```bash
go install github.com/vedantlavale/taasant/cmd/taasant@latest
```

This puts `taasant` in `~/go/bin`. Add that folder to your `PATH` if it is not there already.

### Build from source

```bash
git clone https://github.com/vedantlavale/taasant.git
cd taasant
go build -o taasant ./cmd/taasant
sudo mv taasant /usr/local/bin/
```

## Setup

You need a Telegram bot token, a chat ID, and an encryption key.

### 1. Create a Telegram bot

Message [@BotFather](https://t.me/BotFather) on Telegram, send `/newbot`, and follow the instructions. BotFather will give you a token similar to:

```text
123456:ABC...
```

### 2. Start a chat with the bot

Open the new bot and press **Start**. The bot must have permission to send messages to the chat you use for storage.

### 3. Find your chat ID

Message [@userinfobot](https://t.me/userinfobot). It will reply with your numeric chat ID.

### 4. Generate an encryption key

```bash
taasant keygen
```

Save the output somewhere secure. It is a 256-bit encryption key and cannot be recovered if lost.

### 5. Configure the environment

Add these values to `~/.zshrc` or `~/.bashrc`:

```bash
export TG_BOT_TOKEN="123456:ABC..."
export TG_CHAT_ID="1234567890"
export TAAS_KEY="the-64-character-value-from-keygen"
```

Then reload your shell:

```bash
source ~/.zshrc
```

Use the same key every time. Files encrypted with a different key cannot be downloaded.

## Commands

| Command                                       | Description                                                                  |
| --------------------------------------------- | ---------------------------------------------------------------------------- |
| `taasant keygen`                              | Generate and print a new encryption key.                                     |
| `taasant upload <file>`                       | Encrypt and upload a file.                                                   |
| `taasant list [search]`                       | List uploaded files, newest first. An optional search term filters the list. |
| `taasant download <name-or-id> [output-file]` | Download a file. By default, it uses the original filename.                  |
| `taasant delete <name-or-id>`                 | Delete a file from Telegram and remove it from the local index.              |

### Finding files

You do not need to type a complete filename. `taasant` tries these searches in order:

1. Exact filename, such as `holiday.jpg`
2. A case-insensitive part of the filename, such as `holi`
3. Letters appearing in order, such as `hjp` matching `holiday.jpg`

If multiple files match, `taasant` shows the matches and stops instead of guessing. You can then type more of the filename or use the full ID.

### Downloads never overwrite files

If the output filename already exists, the download fails rather than replacing it:

```bash
taasant download holiday.jpg holiday-copy.jpg
```

## Where data is stored

| Data                               | Location                                          |
| ---------------------------------- | ------------------------------------------------- |
| Encrypted file parts and manifests | Your Telegram chat                                |
| Local file index                   | `index.json` in your user configuration directory |
| Index backup ID                    | `index.json.remote` beside the local index        |
| Telegram token and encryption key  | The environment variables you configure           |

The default index locations are usually:

- macOS: `~/Library/Application Support/taasant/index.json`
- Linux: `~/.config/taasant/index.json`

You can choose another index location with:

```bash
export TAAS_INDEX="/path/to/index.json"
```

The backup index is encrypted with `TAAS_KEY`. Keep the `.remote` file with your local index, or copy its ID somewhere safe as well; the ID is needed to locate that backup in Telegram.

## Security and limitations

- Keep `TAAS_KEY` private. Anyone who has the key and the relevant Telegram IDs may be able to decrypt files.
- Telegram receives encrypted bytes, not the original file contents.
- Losing the key makes the stored files unrecoverable.
- Losing the local index does not delete your Telegram files, but it makes their IDs difficult to find. Keep a copy of the index and its remote backup ID.
- Telegram only allows bots to delete messages sent within the last 48 hours.
- Large uploads use memory because parts are encrypted before being sent.
- Files downloaded manually from Telegram appear as encrypted `part.bin` files and cannot be opened directly.

## Troubleshooting

| Message                                       | What to check                                               |
| --------------------------------------------- | ----------------------------------------------------------- |
| `TG_BOT_TOKEN is not set`                     | Set `TG_BOT_TOKEN` in your shell.                           |
| `TG_CHAT_ID must be a number`                 | Check that `TG_CHAT_ID` contains only a numeric chat ID.    |
| `401 Unauthorized`                            | The bot token may be wrong or revoked.                      |
| `the bot can't send messages to the bot`      | You may have used the bot's ID instead of your own chat ID. |
| `bot can't initiate conversation with a user` | Open the bot chat and press **Start**.                      |
| `cipher: message authentication failed`       | The `TAAS_KEY` does not match the key used for the upload.  |
| `file exists`                                 | Choose a different download output filename.                |

## Development

Run the tests with:

```bash
go test ./...
```

To check for data races:

```bash
go test -race ./...
```

The main directories are:

- `cmd/taasant/` — command-line handling and the local index
- `tgstore/` — encryption, file splitting, concurrent transfers, and Telegram API calls

## License and credits

The storage design was inspired by [golang-design/tgstore](https://github.com/golang-design/tgstore). `taasant` is a small rewrite using only Go's standard library.
