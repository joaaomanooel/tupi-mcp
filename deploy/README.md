# Implantação do Tupi MCP com OctoPrint

Este diretório contém a configuração versionada de implantação do Tupi MCP ao lado de uma instância oficial do OctoPrint. O OctoPrint fica na versão estável `1.11.8` e o Tupi MCP usa uma tag de release publicada, nunca `latest`. As duas portas publicadas escutam apenas no loopback do host, e todo o tráfego entre o Tupi MCP e o OctoPrint fica em uma rede Docker dedicada.

## Pré-requisitos

- Raspberry Pi `arm64` com Docker Engine e o plugin `docker compose` v2.
- A impressora conectada ao host, criando `/dev/serial/by-id/usb-1a86_USB_Serial-if00-port0`.
- O grupo `dialout` do host com GID `20`, que é repassado ao contêiner do OctoPrint. Confirme com `getent group dialout`.
- Uma tag de release publicada no GHCR para `ghcr.io/joaaomanooel/tupi-mcp`, no formato `vMAJOR.MINOR.PATCH`. Se ainda não houver release publicado, publique um antes de implantar: este incremento não compila imagens.
- Aproximadamente 500 MB livres em disco para a imagem do OctoPrint e memória suficiente para o conjunto. O Pi compartilha cerca de 900 MiB com outros serviços, e esta configuração não impõe limites de memória; verifique o espaço livre antes de subir os contêineres.

## Arquivos

| Arquivo | Papel |
| --- | --- |
| `compose.yaml` | Serviços `octoprint` e `tupi-mcp`, rede `octoprint-api` e volume `octoprint-data` |
| `env.example` | Nomes das variáveis e placeholders, sem valores utilizáveis |
| `.env` | Segredos reais do host, ignorado pelo Git, criado a partir de `env.example` |

## Preparação do `.env`

O Compose carrega `.env` do diretório do arquivo de composição, então execute os comandos a partir de `deploy/`.

```sh
cp env.example .env
chmod 600 .env
openssl rand -hex 32
```

Substitua os três valores:

| Variável | Conteúdo |
| --- | --- |
| `TUPI_MCP_IMAGE_TAG` | Tag `vMAJOR.MINOR.PATCH` publicada no GHCR |
| `TUPI_MCP_TOKEN` | Token gerado por `openssl rand -hex 32`, com pelo menos 32 bytes e sem espaços |
| `TUPI_OCTOPRINT_API_KEY` | Chave de API de um usuário dedicado somente leitura |

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

`config -q` falha se alguma variável obrigatória estiver ausente ou se a sintaxe do arquivo estiver inválida. `--images` deve mostrar `octoprint/octoprint:1.11.8` e a tag escolhida para o Tupi MCP.

## Implantação no host

```sh
cd deploy
docker compose pull
docker compose up -d
```

`up -d` inicia os dois serviços, mas não envia arquivos nem comandos à impressora. O OctoPrint sobe sem configuração de porta serial: selecione em `Settings > Serial Connection` a porta `/dev/serial/by-id/usb-1a86_USB_Serial-if00-port0`, que é o caminho mapeado do host, e conecte manualmente quando o hardware estiver pronto.

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
| `pull` recusa a imagem do Tupi MCP | `TUPI_MCP_IMAGE_TAG` não aponta para uma tag publicada |
| O Tupi MCP encerra na inicialização | Token com menos de 32 bytes, com espaços ou variável obrigatória ausente |
| Ferramentas retornam erro seguro do OctoPrint | Chave inválida, usuário sem o grupo `Read-only Access` ou serviço `octoprint` fora do ar |
| `docker compose config` reclama de variável ausente | O comando não foi executado em `deploy/` ou o `.env` não foi criado |
