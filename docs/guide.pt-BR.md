<p align="center">
  <img src="assets/finnick-header.svg#gh-light-mode-only" alt="Finnick" width="320">
  <img src="assets/finnick-header-dark.svg#gh-dark-mode-only" alt="Finnick" width="320">
</p>

# Guia: secrets com SOPS, age e Finnick

[English](guide.md) · **Português**

Este guia leva você de um repositório sem nada até um fluxo completo: secrets cifrados no próprio repo,
cada pessoa com a sua chave, CI decifrando no build e o Finnick mostrando tudo numa interface.

- [Como funciona](#como-funciona)
- [1. Instale as ferramentas](#1-instale-as-ferramentas)
- [2. Crie a sua chave age](#2-crie-a-sua-chave-age)
- [3. Diga quem decifra o quê (`.sops.yaml`)](#3-diga-quem-decifra-o-quê-sopsyaml)
- [4. Crie os arquivos de secrets](#4-crie-os-arquivos-de-secrets)
- [5. O dia a dia com o sops](#5-o-dia-a-dia-com-o-sops)
- [6. Use o Finnick](#6-use-o-finnick)
- [7. Dê acesso a alguém](#7-dê-acesso-a-alguém)
- [8. Tire o acesso de alguém](#8-tire-o-acesso-de-alguém)
- [9. CI e Docker](#9-ci-e-docker)
- [10. Conflitos no git](#10-conflitos-no-git)
- [11. Problemas comuns](#11-problemas-comuns)

## Como funciona

- **age** gera um par de chaves por pessoa (ou por CI). A chave **pública** (`age1…`) pode circular à vontade.
  A **privada** fica só na máquina da pessoa.
- **SOPS** cifra os **valores** de um arquivo (`.env`, YAML, JSON…) e deixa os **nomes** legíveis. Cada arquivo
  tem uma chave de dados própria, cifrada uma vez para cada chave pública autorizada. Qualquer uma das
  privadas correspondentes consegue abrir o arquivo.
- **`.sops.yaml`** diz quais chaves públicas recebem acesso a quais arquivos.
- **Finnick** é uma interface local por cima disso: lista os arquivos, mostra e copia valores, e mostra
  quem tem acesso a quê e o que está fora de sincronia.

Os arquivos cifrados vão para o git junto com o código. Uma mudança de código que precisa de uma variável
nova vira um commit só.

```text
secrets/dev.env (no git)                       quem abre
API_URL=ENC[AES256_GCM,data:…]                 qualquer chave listada
TOKEN=ENC[AES256_GCM,data:…]                   no .sops.yaml para este arquivo
sops_age__list_0__map_recipient=age1…ana
sops_age__list_1__map_recipient=age1…bia
```

## 1. Instale as ferramentas

| Sistema | Comando |
| --- | --- |
| macOS | `brew install sops age` |
| Debian/Ubuntu | `sudo apt install age` e o `sops` pelo [release oficial](https://github.com/getsops/sops/releases) (abaixo) |
| Arch | `sudo pacman -S sops age` |
| Windows | `scoop install sops age` (ou use o WSL2 e siga o Linux) |

`sops` no Debian/Ubuntu:

```bash
SOPS_VERSION=3.13.3
ARCH=$(dpkg --print-architecture)   # amd64 ou arm64
curl -fsSLo /tmp/sops "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${ARCH}"
sudo install -m 755 /tmp/sops /usr/local/bin/sops
```

**Finnick.** No macOS:

```bash
brew install --cask zipluciano/tap/finnick
```

No Linux e no Windows, baixe o arquivo do seu sistema em
[Releases](https://github.com/zipluciano/finnick/releases); o [README](../README.md#install) tem os
comandos.

O Finnick é um binário único: não precisa do `sops` nem do `age` instalados para funcionar.

## 2. Crie a sua chave age

Gere a chave no lugar onde o sops procura por padrão. Assim nada precisa ser configurado no shell:

```bash
# macOS
mkdir -p ~/Library/Application\ Support/sops/age
age-keygen -o ~/Library/Application\ Support/sops/age/keys.txt

# Linux
mkdir -p ~/.config/sops/age
age-keygen -o ~/.config/sops/age/keys.txt
```

O comando imprime a chave pública (`Public key: age1…`). Para vê-la de novo, rode `age-keygen -y <caminho do keys.txt>`.

- **Faça backup** do `keys.txt` no seu gerenciador de senhas. Se perder o arquivo, você perde o acesso, e alguém
  terá de autorizar uma chave nova.
- **Nunca** envie nem versione o `keys.txt`. Envie só a linha `age1…`.
- Já tem a chave em outro lugar? Aponte para ela com `export SOPS_AGE_KEY_FILE=/caminho/keys.txt` no seu shell.

## 3. Diga quem decifra o quê (`.sops.yaml`)

Na raiz do repositório:

```yaml
# .sops.yaml
keys:
  - &ana age1qyq…ana     # cada pessoa ou CI uma vez, com um nome (âncora)
  - &bia age1zxs…bia
  - &ci_deploy age1m4…ci

creation_rules:
  # A PRIMEIRA regra que casar vence: as específicas em cima.
  - path_regex: ^secrets/prod\.env$
    age: [*ana, *ci_deploy]

  - path_regex: ^secrets/[^/]+\.env$
    age: [*ana, *bia]
```

- O `path_regex` é comparado com o caminho **relativo à pasta do `.sops.yaml`**. Rode os comandos do sops a
  partir da raiz do repositório.
- Os nomes das âncoras (`&ana`) são o que o Finnick mostra na interface.
- O `.sops.yaml` só tem chaves **públicas**: versione-o normalmente.

## 4. Crie os arquivos de secrets

**Arquivo novo:** o sops abre o seu editor com um exemplo. Troque pelo conteúdo real, salve e feche; o arquivo
é gravado já cifrado.

```bash
sops edit secrets/dev.env
```

**Importando um `.env` que você já tem:**

```bash
cp .env secrets/dev.env
sops encrypt -i secrets/dev.env   # cifra no lugar, usando a regra do .sops.yaml
rm .env
```

Confira e versione:

```bash
head -3 secrets/dev.env           # nomes legíveis, valores ENC[…]
git add .sops.yaml secrets/
git commit -m "chore: secrets cifrados com SOPS"
```

Deixe o `.env` em texto puro no `.gitignore` (`.env`, `.env.*`) e nunca commite um arquivo de `secrets/` que
não tenha a linha `sops_mac=ENC[`. A seção [CI](#9-ci-e-docker) mostra como o CI pode barrar isso.

## 5. O dia a dia com o sops

| Para | Comando |
| --- | --- |
| Editar (abre o editor, cifra ao salvar) | `sops edit secrets/dev.env` |
| Ver tudo decifrado no terminal | `sops decrypt secrets/dev.env` |
| Ler um valor | `sops decrypt --extract '["API_URL"]' secrets/dev.env` |
| Criar ou alterar um valor sem abrir editor | `sops set secrets/dev.env '["API_URL"]' '"https://api.exemplo"'` |
| Remover uma chave | `sops unset secrets/dev.env '["API_URL"]'` |
| Rodar um comando com as variáveis no ambiente | `sops exec-env secrets/dev.env 'npm run dev'` |
| Ver só os nomes (não precisa de chave) | `grep -o '^[A-Za-z_][A-Za-z0-9_]*' secrets/dev.env \| grep -v '^sops_'` |

O `sops exec-env` injeta as variáveis só no processo e não grava nada em disco. É o jeito recomendado de rodar
o app:

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
# Python, Go, qualquer coisa:
sops exec-env secrets/dev.env 'uv run app serve'
```

Evite `sops decrypt secrets/dev.env > .env`. Se precisar mesmo de um arquivo, garanta que ele está no
`.gitignore` e apague quando terminar.

## 6. Use o Finnick

De qualquer pasta dentro do repositório:

```bash
finnick ui    # abre o navegador
finnick ls    # o mesmo resumo, no terminal
```

Ele acha o `.sops.yaml` subindo pelas pastas, como o sops faz, e usa a mesma chave que o sops. O servidor
escuta só em `127.0.0.1`, e o link que o comando abre vale só para aquela sessão. Feche com `Ctrl+C`.

O que você encontra na interface:

- **Barra lateral:** os arquivos cifrados, agrupados por pasta. O cadeado aberto indica que você consegue
  decifrar o arquivo. O ponto amarelo indica arquivo fora de sincronia com o `.sops.yaml`.
- **Aba Secrets:** os nomes das chaves (o SOPS os guarda em claro, então aparecem mesmo sem acesso). Para ver
  os valores, use **Revelar valores**: a decifração acontece na hora, com verificação de integridade, e os
  valores somem sozinhos após 2 minutos. Cada linha tem um botão de copiar, e há um filtro por nome.
- **Aba Acesso:** quem consegue decifrar o arquivo, lido do próprio arquivo e não só do `.sops.yaml`.
  - **tem acesso**: está no arquivo e na regra.
  - **aguardando updatekeys**: está na regra, mas o arquivo ainda não foi cifrado de novo para essa pessoa.
  - **removido da regra**: ainda consegue decifrar, mas saiu do `.sops.yaml`.
- **Pessoas e chaves:** uma matriz de todas as chaves contra todos os arquivos. É a forma mais rápida de
  responder "quem lê produção?".
- **Regras:** as `creation_rules` na ordem em que o sops aplica, os arquivos que cada uma cobre e os arquivos
  que nenhuma regra cobre.
- **Rodapé:** tema (sistema, claro, escuro) e idioma (English, Português, Español). As escolhas ficam salvas
  em `~/.config/finnick/settings.json`.

Por enquanto o Finnick é **somente leitura**: para editar, use `sops edit` e depois clique em reescanear
(↻). Edição, gestão de pessoas e git pela interface estão no [roadmap](roadmap.md).

## 7. Dê acesso a alguém

1. A pessoa segue o [passo 2](#2-crie-a-sua-chave-age) e envia a chave pública `age1…`.
2. Você adiciona a chave em `keys`, com um nome, e coloca esse nome nas regras dos ambientes que a pessoa pode
   ler:
   ```yaml
   keys:
     - &ana age1qyq…ana
     - &bia age1zxs…bia
     - &caio age1p0d…caio        # novo
   creation_rules:
     - path_regex: ^secrets/prod\.env$
       age: [*ana, *ci_deploy]
     - path_regex: ^secrets/[^/]+\.env$
       age: [*ana, *bia, *caio]  # caio só nos ambientes que não são prod
   ```
3. Abra o Finnick: em **Pessoas e chaves**, o caio aparece como **aguardando updatekeys**. Mudar o
   `.sops.yaml` sozinho não dá acesso a ninguém.
4. Cifre os arquivos de novo para as chaves atuais (é preciso que você mesmo consiga decifrá-los):
   ```bash
   sops updatekeys -y secrets/*.env
   ```
5. Reescaneie no Finnick: tudo verde. Faça commit e push, e a pessoa já consegue decifrar depois do
   `git pull`.

## 8. Tire o acesso de alguém

1. Remova a chave de `keys` e das regras.
2. Rode `sops updatekeys -y secrets/*.env`. Antes disso, o Finnick mostra a pessoa como **removido da regra**.
3. **Importante:** a pessoa continua conseguindo decifrar as versões antigas que estão no histórico do git. Para
   proteger de verdade, **troque os valores** no provedor (senhas, tokens, chaves de API), grave os novos com
   `sops edit` e gere uma chave de dados nova:
   ```bash
   sops rotate -i secrets/prod.env
   ```
4. Commit e push.

## 9. CI e Docker

**Uma chave para o CI.** Trate o CI como mais uma pessoa:

```bash
age-keygen -o ci.txt                     # anote a chave pública impressa
gh secret set SOPS_AGE_KEY < ci.txt      # guarda a privada como secret do repositório
rm ci.txt
```

Adicione a chave pública ao `.sops.yaml` (por exemplo `&ci_deploy`) só nas regras dos ambientes que o CI
precisa ler, e rode `sops updatekeys -y secrets/*.env`.

**No GitHub Actions:**

```yaml
- name: Instalar sops
  run: |
    curl -fsSLo /tmp/sops https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64
    sudo install -m 755 /tmp/sops /usr/local/bin/sops

- name: Build
  env:
    SOPS_AGE_KEY: ${{ secrets.SOPS_AGE_KEY }}
  run: sops exec-env secrets/prod.env 'npm run build'
```

**Barrar secret em texto puro.** Um passo que falha o PR se algum arquivo de `secrets/` não estiver cifrado:

```yaml
- name: Secrets cifrados com SOPS
  run: |
    for f in secrets/*.env; do
      grep -q '^sops_mac=ENC\[' "$f" || { echo "::error file=$f::$f não está cifrado com SOPS"; exit 1; }
    done
```

**Docker.** Passe o `.env` decifrado como *build secret*. Ele fica montado só durante um `RUN` e nunca vai para
uma camada da imagem:

```dockerfile
# syntax=docker/dockerfile:1.7
RUN --mount=type=secret,id=DOTENV,target=/app/.env.production.local,required=true \
    npm run build
```

```bash
DOTENV="$(sops decrypt secrets/prod.env)" docker build --secret id=DOTENV,env=DOTENV .
```

Coloque `secrets` e `.sops.yaml` no `.dockerignore`: a imagem não precisa deles.

## 10. Conflitos no git

Toda edição feita pelo sops regrava o `sops_mac` e o `sops_lastmodified`. Por isso, duas pessoas que editam o
mesmo arquivo ao mesmo tempo **sempre** geram conflito, mesmo mexendo em chaves diferentes. Não resolva o
conflito à mão dentro do arquivo cifrado: o MAC deixa de bater e o sops recusa o arquivo.

Faça assim:

1. Fique com a versão do outro lado:
   - num `merge`: `git checkout --theirs secrets/dev.env`
   - num `rebase`, os lados se invertem: `git checkout --ours secrets/dev.env`
2. Refaça a sua alteração com `sops edit secrets/dev.env`.
3. `git add secrets/dev.env` e continue o merge ou o rebase.

Para evitar o conflito, rode `git pull` antes de editar e faça commit logo depois. O merge por chave
automático está no roadmap do Finnick.

## 11. Problemas comuns

| Mensagem ou sintoma | Causa e solução |
| --- | --- |
| `Failed to get the data key required to decrypt the SOPS file` | O sops não achou a sua chave, ou ela não é destinatária desse arquivo. Confira o caminho do `keys.txt` ([passo 2](#2-crie-a-sua-chave-age)) e se a sua chave pública está no arquivo: o Finnick mostra isso na aba Acesso. |
| `no matching creation rules found` | O caminho do arquivo não casa com nenhum `path_regex`. Rode o comando a partir da raiz do repo e confira a regex (ela é relativa à pasta do `.sops.yaml`). |
| `MAC mismatch` / `Failed to verify data integrity` | O arquivo foi editado fora do sops (à mão, ou num conflito mal resolvido). Recupere a versão do git (`git checkout -- secrets/dev.env`) e refaça a mudança com `sops edit`. |
| O Finnick mostra um arquivo com cadeado fechado | A sua chave não está entre os destinatários. Peça acesso ([passo 7](#7-dê-acesso-a-alguém)). |
| O Finnick diz que o arquivo está fora de sincronia | O `.sops.yaml` mudou e ninguém rodou `sops updatekeys` nesse arquivo. |
| O `finnick ui` diz que não há repositório | Rode-o dentro de um repositório git ou de uma pasta com `.sops.yaml` (ou passe `--dir`). |

**Lembrete:** variáveis que vão para o navegador (`VITE_*`, `NEXT_PUBLIC_*`) são públicas no bundle, cifradas
ou não. O SOPS protege o repositório, não o que você entrega ao cliente.
