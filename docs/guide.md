<p align="center">
  <img src="assets/finnick-header.svg" alt="Finnick" width="320">
</p>

# Guide: secrets with SOPS, age and Finnick

**English** · [Português](guide.pt-BR.md)

This guide takes you from an empty repository to a complete workflow: secrets encrypted inside the repo,
one key per person, CI decrypting at build time, and Finnick showing it all in a UI.

- [How it works](#how-it-works)
- [1. Install the tools](#1-install-the-tools)
- [2. Create your age key](#2-create-your-age-key)
- [3. Say who decrypts what (`.sops.yaml`)](#3-say-who-decrypts-what-sopsyaml)
- [4. Create the secret files](#4-create-the-secret-files)
- [5. Day to day with sops](#5-day-to-day-with-sops)
- [6. Use Finnick](#6-use-finnick)
- [7. Give someone access](#7-give-someone-access)
- [8. Remove someone's access](#8-remove-someones-access)
- [9. CI and Docker](#9-ci-and-docker)
- [10. Git conflicts](#10-git-conflicts)
- [11. Troubleshooting](#11-troubleshooting)

## How it works

- **age** creates a key pair per person (or per CI). The **public** key (`age1…`) can be shared freely; the
  **private** key never leaves that person's machine.
- **SOPS** encrypts the **values** of a file (`.env`, YAML, JSON…) and keeps the **names** readable. Each file
  has its own data key, encrypted once for every authorized public key; any of the matching private keys
  opens it.
- **`.sops.yaml`** says which public keys get access to which files.
- **Finnick** is a local UI on top of that: it lists the files, reveals and copies values, and shows who
  has access to what and what is out of sync.

The encrypted files are committed with the code. A code change that needs a new variable is one commit.

```text
secrets/dev.env (in git)                       who can open it
API_URL=ENC[AES256_GCM,data:…]                 any key listed in
TOKEN=ENC[AES256_GCM,data:…]                   .sops.yaml for this file
sops_age__list_0__map_recipient=age1…ana
sops_age__list_1__map_recipient=age1…bia
```

## 1. Install the tools

| System | Command |
| --- | --- |
| macOS | `brew install sops age` |
| Debian/Ubuntu | `sudo apt install age`, and `sops` from the [official release](https://github.com/getsops/sops/releases) (below) |
| Arch | `sudo pacman -S sops age` |
| Windows | `scoop install sops age` (or use WSL2 and follow Linux) |

`sops` on Debian/Ubuntu:

```bash
SOPS_VERSION=3.13.3
ARCH=$(dpkg --print-architecture)   # amd64 or arm64
curl -fsSLo /tmp/sops "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${ARCH}"
sudo install -m 755 /tmp/sops /usr/local/bin/sops
```

**Finnick.** On macOS:

```bash
brew install --cask zipluciano/tap/finnick
```

On Linux and Windows, download the archive for your system from
[Releases](https://github.com/zipluciano/finnick/releases); the [README](../README.md#install) has the
commands.

Finnick is a single binary; it does not need `sops` or `age` installed to work.

## 2. Create your age key

Create the key where sops looks for it by default, so nothing has to be configured in your shell:

```bash
# macOS
mkdir -p ~/Library/Application\ Support/sops/age
age-keygen -o ~/Library/Application\ Support/sops/age/keys.txt

# Linux
mkdir -p ~/.config/sops/age
age-keygen -o ~/.config/sops/age/keys.txt
```

The command prints your public key (`Public key: age1…`). Show it again with `age-keygen -y <path to keys.txt>`.

- **Back up** `keys.txt` in your password manager. If you lose it you lose access, and someone has to authorize
  a new key.
- **Never** send or commit `keys.txt`. Share only the `age1…` line.
- Already have a key somewhere else? Point to it with `export SOPS_AGE_KEY_FILE=/path/keys.txt` in your shell.

## 3. Say who decrypts what (`.sops.yaml`)

At the root of the repository:

```yaml
# .sops.yaml
keys:
  - &ana age1qyq…ana     # each person or CI once, with a name (anchor)
  - &bia age1zxs…bia
  - &ci_deploy age1m4…ci

creation_rules:
  # The FIRST matching rule wins: specific rules go first.
  - path_regex: ^secrets/prod\.env$
    age: [*ana, *ci_deploy]

  - path_regex: ^secrets/[^/]+\.env$
    age: [*ana, *bia]
```

- `path_regex` is matched against the path **relative to the folder of `.sops.yaml`**. Run sops commands from
  the repository root.
- The anchor names (`&ana`) are what Finnick shows in the UI.
- `.sops.yaml` only holds **public** keys: commit it like any other file.

## 4. Create the secret files

**New file:** sops opens your editor with an example. Replace it with the real content, save and close; the
file is written already encrypted.

```bash
sops edit secrets/dev.env
```

**Importing an existing `.env`:**

```bash
cp .env secrets/dev.env
sops encrypt -i secrets/dev.env   # encrypts in place, using the .sops.yaml rule
rm .env
```

Check and commit:

```bash
head -3 secrets/dev.env           # readable names, ENC[…] values
git add .sops.yaml secrets/
git commit -m "chore: secrets encrypted with SOPS"
```

Keep plain-text `.env` files in `.gitignore` (`.env`, `.env.*`) and never commit a file under `secrets/` that
lacks the `sops_mac=ENC[` line. The [CI section](#9-ci-and-docker) shows how CI can block that.

## 5. Day to day with sops

| To | Command |
| --- | --- |
| Edit (opens your editor, encrypts on save) | `sops edit secrets/dev.env` |
| Print everything decrypted | `sops decrypt secrets/dev.env` |
| Read one value | `sops decrypt --extract '["API_URL"]' secrets/dev.env` |
| Add or change a value without an editor | `sops set secrets/dev.env '["API_URL"]' '"https://api.example"'` |
| Remove a key | `sops unset secrets/dev.env '["API_URL"]'` |
| Run a command with the variables set | `sops exec-env secrets/dev.env 'npm run dev'` |
| List names only (no key needed) | `grep -o '^[A-Za-z_][A-Za-z0-9_]*' secrets/dev.env \| grep -v '^sops_'` |

`sops exec-env` puts the variables into the process only and writes nothing to disk. It is the recommended
way to run your app:

```json
{
  "scripts": {
    "dev": "sops exec-env secrets/dev.env vite",
    "build:prod": "sops exec-env secrets/prod.env 'npm run build'",
    "secrets": "finnick ui"
  }
}
```

```bash
# Python, Go, anything:
sops exec-env secrets/dev.env 'uv run app serve'
```

Avoid `sops decrypt secrets/dev.env > .env`. If you really need a file, make sure it is in `.gitignore` and
delete it when done.

## 6. Use Finnick

From any folder inside the repository:

```bash
finnick ui    # opens your browser
finnick ls    # the same summary, in the terminal
```

It finds `.sops.yaml` by walking up the folders, like sops, and uses the same key as sops. The server
listens on `127.0.0.1` only, and the link it opens works for that session only. Stop it with `Ctrl+C`.

What you will find:

- **Sidebar:** the encrypted files, grouped by folder. An open padlock means you can decrypt the file. A
  yellow dot means the file is out of sync with `.sops.yaml`.
- **Secrets tab:** the key names (SOPS stores them in clear, so they show even without access). To see values,
  use **Reveal values**: decryption happens on demand, with an integrity check, and values hide again after
  2 minutes. Each row has a copy button, and there is a filter by name.
- **Access tab:** who can decrypt the file, read from the file itself, not only from `.sops.yaml`.
  - **has access**: in the file and in the rule.
  - **pending updatekeys**: in the rule, but the file was not re-encrypted for that person yet.
  - **removed from rule**: can still decrypt, but is no longer in `.sops.yaml`.
- **People & keys:** a matrix of every key against every file. It is the fastest way to answer "who can read
  production?".
- **Rules:** the `creation_rules` in the order sops applies them, which files each one covers, and the files no
  rule covers.
- **Footer:** theme (system, light, dark) and language (English, Português, Español). Choices are saved in
  `~/.config/finnick/settings.json`.

For now Finnick is **read-only**: edit with `sops edit`, then hit rescan (↻). Editing, people management
and git from the UI are on the [roadmap](roadmap.md).

## 7. Give someone access

1. The person follows [step 2](#2-create-your-age-key) and sends you their `age1…` public key.
2. Add the key under `keys` with a name, and put that name in the rules for the environments they may read:
   ```yaml
   keys:
     - &ana age1qyq…ana
     - &bia age1zxs…bia
     - &caio age1p0d…caio        # new
   creation_rules:
     - path_regex: ^secrets/prod\.env$
       age: [*ana, *ci_deploy]
     - path_regex: ^secrets/[^/]+\.env$
       age: [*ana, *bia, *caio]  # caio only outside prod
   ```
3. Open Finnick: under **People & keys**, caio shows as **pending updatekeys**. Changing `.sops.yaml` alone
   gives nobody access.
4. Re-encrypt the files for the current keys (you must be able to decrypt them yourself):
   ```bash
   sops updatekeys -y secrets/*.env
   ```
5. Rescan in Finnick: all green. Commit and push; after `git pull` the person can decrypt.

## 8. Remove someone's access

1. Remove the key from `keys` and from the rules.
2. Run `sops updatekeys -y secrets/*.env`. Until then, Finnick shows the person as **removed from rule**.
3. **Important:** the person can still decrypt the old versions in git history. To really protect the secrets,
   **change the values** at the provider (passwords, tokens, API keys), store the new ones with `sops edit`,
   and generate a new data key:
   ```bash
   sops rotate -i secrets/prod.env
   ```
4. Commit and push.

## 9. CI and Docker

**A key for CI.** Treat CI as one more person:

```bash
age-keygen -o ci.txt                     # note the public key it prints
gh secret set SOPS_AGE_KEY < ci.txt      # stores the private key as a repository secret
rm ci.txt
```

Add the public key to `.sops.yaml` (for example `&ci_deploy`) only in the rules for the environments CI needs,
then run `sops updatekeys -y secrets/*.env`.

**In GitHub Actions:**

```yaml
- name: Install sops
  run: |
    curl -fsSLo /tmp/sops https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64
    sudo install -m 755 /tmp/sops /usr/local/bin/sops

- name: Build
  env:
    SOPS_AGE_KEY: ${{ secrets.SOPS_AGE_KEY }}
  run: sops exec-env secrets/prod.env 'npm run build'
```

**Block plain-text secrets.** A step that fails the PR if any file under `secrets/` is not encrypted:

```yaml
- name: Secrets encrypted with SOPS
  run: |
    for f in secrets/*.env; do
      grep -q '^sops_mac=ENC\[' "$f" || { echo "::error file=$f::$f is not encrypted with SOPS"; exit 1; }
    done
```

**Docker.** Pass the decrypted `.env` as a *build secret*. It is mounted for a single `RUN` and never ends up in
an image layer:

```dockerfile
# syntax=docker/dockerfile:1.7
RUN --mount=type=secret,id=DOTENV,target=/app/.env.production.local,required=true \
    npm run build
```

```bash
DOTENV="$(sops decrypt secrets/prod.env)" docker build --secret id=DOTENV,env=DOTENV .
```

Add `secrets` and `.sops.yaml` to `.dockerignore`: the image does not need them.

## 10. Git conflicts

Every sops edit rewrites `sops_mac` and `sops_lastmodified`. So two people editing the same file at the same
time **always** conflict, even when they changed different keys. Do not resolve the conflict by hand inside the
encrypted file: the MAC stops matching and sops rejects the file.

Instead:

1. Keep the other side's version:
   - in a `merge`: `git checkout --theirs secrets/dev.env`
   - in a `rebase` the sides are swapped: `git checkout --ours secrets/dev.env`
2. Redo your change with `sops edit secrets/dev.env`.
3. `git add secrets/dev.env` and continue the merge or rebase.

To avoid conflicts, `git pull` before editing and commit right after. Automatic per-key merging is on the
Finnick roadmap.

## 11. Troubleshooting

| Message or symptom | Cause and fix |
| --- | --- |
| `Failed to get the data key required to decrypt the SOPS file` | sops did not find your key, or your key is not a recipient of the file. Check the `keys.txt` location ([step 2](#2-create-your-age-key)) and whether your public key is on the file: Finnick shows it in the Access tab. |
| `no matching creation rules found` | The file path matches no `path_regex`. Run the command from the repository root and check the regex (it is relative to the folder of `.sops.yaml`). |
| `MAC mismatch` / `Failed to verify data integrity` | The file was edited outside sops (by hand, or in a badly resolved conflict). Restore it from git (`git checkout -- secrets/dev.env`) and redo the change with `sops edit`. |
| Finnick shows a file with a closed padlock | Your key is not a recipient. Ask for access ([step 7](#7-give-someone-access)). |
| Finnick says a file is out of sync | `.sops.yaml` changed and nobody ran `sops updatekeys` on that file. |
| `finnick ui` says there is no repository | Run it inside a git repository or a folder with `.sops.yaml` (or pass `--dir`). |

**Reminder:** variables that ship to the browser (`VITE_*`, `NEXT_PUBLIC_*`) are public in the bundle, encrypted
or not. SOPS protects the repository, not what you deliver to the client.
