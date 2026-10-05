# taas

Use a Telegram chat as encrypted file storage, from the command line.

```
$ taas upload holiday.jpg
104:BQACAgUAAxkDAAMG...

$ taas list
2026-10-05 12:09    45.0 MiB  holiday.jpg
    104:BQACAgUAAxkDAAMG...

$ taas download hol
saved holiday.jpg
```

Files are encrypted on your computer before they are sent, so Telegram only ever holds scrambled data. Large files are split into parts automatically. taas is one small binary with no dependencies, written in Go.

Please store only what you need. Don't abuse Telegram's service.

## How it works

1. The file is cut into parts of 19 MiB, because a bot cannot download anything larger than 20 MB.
2. Each part is encrypted with AES-256-GCM and sent to your chat as a message.
3. A small manifest listing the parts is encrypted and sent the same way.
4. The manifest's ID is saved, with the file name, in an index on your computer.

Downloading reverses this: look up the name, fetch the manifest, fetch each part, decrypt, and join.

## Install

You need [Go](https://go.dev/dl/) 1.27 or newer.

```bash
git clone <this repository>
cd tass-ant
go build -o taas ./cmd/taas
sudo mv taas /usr/local/bin/
```

`taas` now works from any folder.

## Setup

You need three values: a bot token, your chat ID and an encryption key.

1. **Create a bot.** In Telegram, message [@BotFather](https://t.me/BotFather), send `/newbot` and follow the steps. It gives you a token like `123456:ABC...`.
2. **Start a chat with your bot.** Open the bot you just created and press Start. A bot can only message people who have messaged it first.
3. **Find your chat ID.** Message [@userinfobot](https://t.me/userinfobot). It replies with your numeric ID.
4. **Generate a key.**

   ```bash
   taas keygen
   ```

5. **Save all three** at the end of `~/.zshrc` (or `~/.bashrc`):

   ```bash
   export TG_BOT_TOKEN="123456:ABC..."
   export TG_CHAT_ID="1234567890"
   export TAAS_KEY="the 64 characters from keygen"
   ```

   Then run `source ~/.zshrc`, or open a new terminal.

Generate the key once and keep a copy somewhere safe, such as a password manager. Files uploaded with one key cannot be read with another.

## Usage

| Command | What it does |
| --- | --- |
| `taas upload <file>` | Store a file. Prints its ID and remembers its name. |
| `taas list [search]` | Show stored files, newest first. |
| `taas download <name or id> [output-file]` | Fetch a file. Saves under its original name unless you give another. |
| `taas delete <name or id>` | Remove a file from the chat and from the index. |
| `taas keygen` | Print a new encryption key. |

### Names

You don't have to type a full name. taas tries three rules and uses the first one that finds anything:

1. The exact name: `holiday.jpg`
2. Part of the name, ignoring case: `holi`
3. The letters in order, with anything between them: `hjp`

If several files match, taas lists them and stops, so it never downloads or deletes the wrong one. A full ID always works too.

`download` never overwrites an existing file. Give a different output name or remove the old file first.

## Where things are kept

| What | Where |
| --- | --- |
| Your files | Your Telegram chat, as encrypted `part.bin` messages |
| Names and IDs | `index.json` in your user config folder (`~/Library/Application Support/taas/` on macOS, `~/.config/taas/` on Linux). Set `TAAS_INDEX` to move it. |
| Token and key | The environment variables you set |

## Good to know

- **Keep your key.** It is not stored anywhere else. Without it, uploaded files cannot be decrypted.
- **Back up the index.** It exists only on this computer. Without it the files are still in the chat, but you no longer know their IDs.
- **Delete has a time limit.** Telegram only lets a bot delete messages sent in the last 48 hours.
- **Large uploads use memory.** Expect a few hundred MB while uploading files larger than 19 MiB.
- **The messages in the chat are not usable by hand.** Downloading a `part.bin` from the Telegram app gives you encrypted bytes. Use `taas download`.

## Troubleshooting

| Error | Cause |
| --- | --- |
| `the bot can't send messages to the bot` | `TG_CHAT_ID` is a bot's ID, often the number at the start of the token. Use your own ID from @userinfobot. |
| `bot can't initiate conversation with a user` | You haven't pressed Start in the chat with your bot. |
| `401 Unauthorized` | `TG_BOT_TOKEN` is wrong or was revoked. |
| `cipher: message authentication failed` | `TAAS_KEY` is not the key the file was uploaded with. |
| `file exists` | `download` won't overwrite. Give another output name. |

## Development

```bash
go test ./...          # runs against a fake Telegram server, no token needed
go test -race ./...    # also checks for concurrency mistakes
```

| Path | Contents |
| --- | --- |
| `cmd/taas/` | The command line and the name index |
| `tgstore/` | Upload, download and delete; encryption; the Telegram calls |

## Credits

The storage idea comes from [golang-design/tgstore](https://github.com/golang-design/tgstore). taas is a smaller rewrite using only the Go standard library, built to learn Go.
