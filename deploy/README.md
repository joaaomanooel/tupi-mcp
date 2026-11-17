# Implantação do Tupi MCP com OctoPrint

Este diretório contém a configuração versionada de implantação do Tupi MCP ao lado de uma instância oficial do OctoPrint. O OctoPrint fica na versão estável `1.11.8` e o Tupi MCP fica fixo na tag estável `v0.1.0` de `ghcr.io/joaaomanooel/tupi-mcp`, nunca `latest`; o Pi não compila imagens. A configuração é versionada, mas a implantação no Raspberry Pi e a validação com hardware real ainda não foram realizadas. As duas portas publicadas escutam apenas no loopback do host, e todo o tráfego entre o Tupi MCP e o OctoPrint fica em uma rede Docker dedicada.

## Pré-requisitos

- Raspberry Pi `arm64` com Docker Engine e o plugin `docker compose` v2.
- A impressora conectada ao host, criando `/dev/serial/by-id/usb-1a86_USB_Serial-if00-port0`.
- O grupo `dialout` do host com GID `20`, que é repassado ao contêiner do OctoPrint. Confirme com `getent group dialout`.
- A imagem `ghcr.io/joaaomanooel/tupi-mcp:v0.1.0` publicada no GHCR. Ela só existe depois que o job de publicação de um release estável termina com sucesso; veja a seção Imagem e sequência de release. Este incremento não compila imagens.
- Aproximadamente 500 MB livres em disco para a imagem do OctoPrint e memória suficiente para o conjunto. O Pi compartilha cerca de 900 MiB com outros serviços, e esta configuração não impõe limites de memória; verifique o espaço livre antes de subir os contêineres.

## Arquivos

| Arquivo | Papel |
| --- | --- |
| `compose.yaml` | Serviços `octoprint` e `tupi-mcp`, rede `octoprint-api` e volume `octoprint-data` |
| `env.example` | Nomes das variáveis e placeholders, sem valores utilizáveis |
| `.env` | Segredos reais do host, ignorado pelo Git, criado a partir de `env.example` |

## Imagem e sequência de release

`compose.yaml` fixa `ghcr.io/joaaomanooel/tupi-mcp:v0.1.0`. A tag não vem do `.env`, e o Pi não compila imagens.

A imagem versionada só passa a existir quando o job de publicação termina com sucesso. A sequência é:

1. Faça o merge da mudança em `main`.
2. Crie a tag estável `vMAJOR.MINOR.PATCH` apontando para um commit contido em `main`, como `v0.1.0`.
3. Confira o workflow acionado pelo push da tag: ele valida o código e compila e escaneia as imagens `amd64`, `arm64` e `arm/v7`, sem publicá-las. Essa é uma verificação manual antes do release; o job de publicação não exige que ela tenha passado.
4. Publique um release estável, não uma pré-release, a partir da tag, com notas não vazias. O job de publicação então recompila as três imagens, escaneia exatamente esses digests e só depois cria o manifest multiarch com a tag exata do release, por exemplo `ghcr.io/joaaomanooel/tupi-mcp:v0.1.0`.

Até o job de publicação terminar com sucesso, a tag não existe no registry e `docker compose pull` falha com `manifest unknown`. Aguarde o job; não use `latest`, release candidate ou compilação local no Pi como substituto.

## Preparação do `.env`

O Compose carrega `.env` do diretório do arquivo de composição, então execute os comandos a partir de `deploy/`.

```sh
cp env.example .env
chmod 600 .env
openssl rand -hex 32
```

Substitua os dois valores:

| Variável | Conteúdo |
| --- | --- |
| `TUPI_MCP_TOKEN` | Token gerado por `openssl rand -hex 32`, com pelo menos 32 bytes e sem espaços |
| `TUPI_OCTOPRINT_API_KEY` | Chave de API de um usuário dedicado somente leitura |

A imagem não é configurada no `.env`: `compose.yaml` fixa a tag `v0.1.0`.

O placeholder de `TUPI_MCP_TOKEN` tem menos de 32 bytes de propósito: se for mantido, o serviço encerra na inicialização com erro seguro em vez de subir com um token fraco.

## Chave do OctoPrint

A chave de API deve pertencer a um usuário dedicado, criado em `Settings > Access Control > Users` e atribuído ao grupo interno `Read-only Access`. Crie a chave estando autenticado como esse usuário e não reutilize a chave de uma conta administradora. Assim, mesmo que a chave seja usada indevidamente, o OctoPrint recusa qualquer operação de escrita, upload ou impressão.

O Tupi MCP não cria, altera ou apaga arquivos no OctoPrint: o cliente HTTP interno emite apenas as requisições GET aprovadas descritas no README do projeto.

## Validação estática

Com o `.env` real presente, sem subir nada:

```sh
cd deploy
docker compose config -q
docker compose config --images
docker compose config --services
```

`config -q` falha se alguma variável obrigatória estiver ausente ou se a sintaxe do arquivo estiver inválida. `--images` deve mostrar `octoprint/octoprint:1.11.8` e `ghcr.io/joaaomanooel/tupi-mcp:v0.1.0`.

## Acesso ao GHCR

A visibilidade do pacote não pode ser deduzida da visibilidade do repositório. Depois de publicar, inspecione o pacote `tupi-mcp` na página de pacotes do GitHub e confirme se o acesso é público ou privado. Não altere a visibilidade do pacote como parte deste incremento.

Se o pacote estiver privado ou exigir login, autentique-se com um token de acesso pessoal (classic) que tenha apenas o escopo `read:packages`:

```sh
docker login ghcr.io --username SEU_USUARIO
```

`SEU_USUARIO` é um exemplo: substitua pelo seu nome de usuário do GitHub, sem os sinais `<` e `>`. Entre `<` e `>` o shell interpretaria o texto como redirecionamento de entrada e o comando não seria executado.

Digite o token no prompt interativo. Não coloque o token no `.env`, em argumentos de linha de comando nem no Git.

O `docker login` grava a credencial no diretório do usuário que executa o comando: para o usuário atual, em `~/.docker/config.json`; com `sudo`, em `/root/.docker/config.json`. Use o mesmo usuário no login, no `pull` e no `up`. Se for realmente necessário usar `sudo` no Docker, autentique-se também com `sudo docker login ghcr.io --username SEU_USUARIO`.

Após o pull, encerre a sessão com `docker logout ghcr.io`, no mesmo usuário e com o mesmo `sudo` do login.

## Implantação no host

```sh
cd deploy
docker compose pull
docker compose up -d
```

`pull` só funciona depois que a tag existe no registry e, se o pacote exigir login, após o `docker login` descrito acima. `up -d` inicia os dois serviços, mas não envia arquivos nem comandos à impressora. O OctoPrint sobe sem configuração de porta serial: selecione em `Settings > Serial Connection` a porta `/dev/serial/by-id/usb-1a86_USB_Serial-if00-port0`, que é o caminho mapeado do host, e conecte manualmente quando o hardware estiver pronto.

## Verificação

```sh
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5000/
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/mcp
docker compose logs --tail=50 tupi-mcp
```

A primeira chamada deve responder com uma página do OctoPrint, e a segunda deve responder `401`, porque a requisição não traz token. Os logs do Tupi MCP não devem conter chave de API, token nem cabeçalhos.

O cliente MCP conecta em `http://127.0.0.1:8080/mcp` com `Authorization: Bearer <TUPI_MCP_TOKEN>`. `TUPI_MCP_ALLOWED_HOSTS` aceita apenas `127.0.0.1:8080` e `localhost:8080`, e nenhuma origem é aceita, então clientes de navegador que enviam `Origin` recebem recusa.

Para usar de outra máquina, encaminhe a porta por SSH em vez de publicar a porta no host:

```sh
ssh -N -L 8080:127.0.0.1:8080 joao@server.local
```

## Segurança e limites

- As portas `5000` e `8080` são publicadas apenas em `127.0.0.1`.
- `octoprint-api` é uma rede Docker dedicada; somente `octoprint` e `tupi-mcp` participam dela.
- O tráfego do Tupi MCP para o OctoPrint usa HTTP sem TLS dentro dessa rede, por isso `TUPI_OCTOPRINT_ALLOW_INSECURE_HTTP=true` é obrigatório e aceitável apenas nesse isolamento. A chave de API circula em claro dentro da rede.
- O contêiner do Tupi MCP roda com sistema de arquivos somente leitura, sem capabilities e sem ganho de privilégios.
- O dispositivo serial é mapeado apenas para o OctoPrint. O Tupi MCP não tem acesso a ele.
- Nenhuma operação de escrita está disponível: sem upload, impressão, pausa, cancelamento, troca de temperatura, movimento de eixos ou G-code arbitrário.

## Solução de problemas

| Sintoma | Verificação |
| --- | --- |
| O OctoPrint não abre a porta serial | `ls -l /dev/serial/by-id/` no host e `getent group dialout`; o GID precisa ser `20` |
| `permission denied` ao abrir a porta | O grupo `20` não existe no contêiner ou o nó mapeado está diferente do configurado no OctoPrint |
| `pull` falha com `manifest unknown` | A tag `v0.1.0` ainda não existe; o job de publicação do release estável não terminou com sucesso |
| `pull` falha com `unauthorized` ou `denied` | Pacote privado ou credencial ausente; confirme a visibilidade real do pacote e repita o `docker login ghcr.io` com `read:packages` usando o mesmo usuário do `pull` |
| O Tupi MCP encerra na inicialização | Token com menos de 32 bytes, com espaços ou variável obrigatória ausente |
| Ferramentas retornam erro seguro do OctoPrint | Chave inválida, usuário sem o grupo `Read-only Access` ou serviço `octoprint` fora do ar |
| `docker compose config` reclama de variável ausente | O comando não foi executado em `deploy/` ou o `.env` não foi criado |
