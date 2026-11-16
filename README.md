# Tupi MCP

O Tupi MCP, no repositório `tupi-mcp`, é um serviço em Go em desenvolvimento para expor informações do OctoPrint em modo somente leitura a clientes MCP.

## Estado atual

O cliente HTTP interno de leitura do OctoPrint, o mapeamento de suas respostas e as ferramentas MCP estão implementados. O cliente faz apenas requisições GET aos endpoints aprovados, valida JSON e os limites de resposta e protege a chave de API. Ele retorna tipos internos para conexão, estado da impressora e temperaturas disponíveis, trabalho atual e progresso, estimativas e lista de arquivos. Campos opcionais ausentes permanecem ausentes; respostas sem trabalho atual e listas vazias são válidas. Campos desconhecidos são ignorados, e tipos incompatíveis em campos conhecidos retornam erros seguros.

O servidor MCP usa o SDK oficial em Go (`github.com/modelcontextprotocol/go-sdk`) e expõe MCP Streamable HTTP em `/mcp`. As quatro ferramentas são somente leitura e não aceitam argumentos:

| Ferramenta | Requisições ao OctoPrint |
| --- | --- |
| `printer_get_status` | `GET /api/connection` e `GET /api/printer` |
| `printer_get_temperatures` | `GET /api/printer` |
| `printer_get_job` | `GET /api/job` |
| `printer_list_files` | `GET /api/files` |

Os esquemas de saída declaram como opcionais todos os campos que podem faltar nas respostas do OctoPrint. Erros de ferramenta retornam uma mensagem segura, sem chave de API, token MCP, corpo de resposta, endereço de rede ou caminho interno, e não encerram o servidor.

A URL base do OctoPrint aceita HTTPS. HTTP é permitido apenas com IP literal de loopback, como `http://127.0.0.1:5000` ou `http://[::1]:5000`; nomes como `localhost` não são aceitos em HTTP. A validação não faz resolução DNS. Respostas de erro são descartadas até o limite configurado de bytes, preservando o erro seguro de status HTTP.

Ainda não estão implementados: implantação, configuração do Raspberry Pi, serviço mock de longa duração, validação com hardware e a decisão final de autenticação e transporte no ADR. A licença do projeto permanece pendente. O MVP não está concluído.

As verificações automatizadas atuais usam mocks locais com `httptest`. Esses resultados não comprovam compatibilidade com uma instância real do OctoPrint nem com Codex ou OpenCode.

## Servidor MCP

O processo escuta em loopback por padrão e encerra de forma graciosa ao receber `SIGINT` ou `SIGTERM`, fechando as sessões MCP ativas. O servidor limita o corpo das requisições, a leitura do cabeçalho a cinco segundos, a leitura completa da requisição a 30 segundos e a espera entre requisições a dois minutos. Ele não define `WriteTimeout` para permitir respostas MCP com Server-Sent Events.

Gere o token MCP localmente com `openssl rand -hex 32`. Ele deve ter pelo menos 32 bytes. Não o salve no repositório.

Execução local com variáveis de ambiente (substitua os valores entre `<>`):

```sh
TUPI_OCTOPRINT_BASE_URL='https://octoprint.example.internal' \
TUPI_OCTOPRINT_API_KEY='<chave-do-octoprint>' \
TUPI_MCP_TOKEN='<token-forte-gerado-localmente>' \
go run ./cmd/tupi-mcp
```

O cliente MCP se conecta a `http://127.0.0.1:8080/mcp` e envia `Authorization: Bearer <token-forte-gerado-localmente>`. O token MCP é apresentado apenas ao serviço; ele nunca é repassado ao OctoPrint nem gravado em log.

### Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
| --- | --- | --- | --- |
| `TUPI_MCP_TOKEN` | sim | — | Token bearer de pelo menos 32 bytes, exigido de todo cliente MCP |
| `TUPI_MCP_LISTEN_ADDRESS` | não | `127.0.0.1:8080` | Endereço `host:porta` de escuta |
| `TUPI_MCP_ALLOW_NON_LOOPBACK` | não | `false` | Libera escuta fora do loopback |
| `TUPI_MCP_ALLOWED_HOSTS` | condicional | derivado do endereço de escuta em loopback | Lista exata de valores de `Host` aceitos |
| `TUPI_MCP_ALLOWED_ORIGINS` | não | vazio, nenhuma origem aceita | Lista exata de origens `http` ou `https` aceitas |
| `TUPI_MCP_MAX_REQUEST_BODY_BYTES` | não | `65536` | Limite do corpo das requisições MCP |
| `TUPI_OCTOPRINT_BASE_URL` | sim | — | URL base do OctoPrint |
| `TUPI_OCTOPRINT_API_KEY` | sim | — | Chave da API do OctoPrint |
| `TUPI_OCTOPRINT_TIMEOUT` | não | `10s` | Timeout das requisições ao OctoPrint, como `10s` |
| `TUPI_OCTOPRINT_MAX_RESPONSE_BYTES` | não | `1048576` | Limite de bytes das respostas do OctoPrint |

`TUPI_MCP_ALLOWED_HOSTS` é obrigatória quando o endereço de escuta não é loopback, e `TUPI_MCP_TOKEN` não pode conter espaços. Valores ausentes, vazios ou inválidos em variáveis obrigatórias encerram a inicialização com erro seguro, sem exibir o valor rejeitado.

`TUPI_MCP_ALLOWED_ORIGINS` filtra o cabeçalho `Origin`; não habilita CORS. Clientes de navegador que dependem de requisições cross-origin não são suportados neste incremento.

### Proposta de autenticação e validação de Host e Origin

O ADR do projeto ainda marca as escolhas de autenticação e transporte como pendentes. A implementação atual adota a seguinte proposta mínima, sujeita a revisão:

- Autenticação por token bearer estático, distinto da chave de API do OctoPrint, configurado apenas por variável de ambiente.
- Token obrigatório com pelo menos 32 bytes e sem whitespace; a comparação é feita em tempo constante sobre resumos SHA-256.
- Requisições sem `Authorization: Bearer <token>`, com esquema diferente, com token incorreto ou com cabeçalhos duplicados recebem `401`, sem detalhes na resposta.
- `Host` é validado em toda requisição, inclusive em caminhos inexistentes, por comparação exata com a lista configurada.
- `Origin` é validado quando enviado, por comparação exata com a lista configurada; sem lista, qualquer `Origin` é recusado. `Origin: null`, origens com caminho ou com múltiplos cabeçalhos são recusados.
- Não há curingas em `Host` nem em `Origin`; cada valor aceito precisa estar listado explicitamente.
- O listener é loopback por padrão. Escutar em outra interface exige `TUPI_MCP_ALLOW_NON_LOOPBACK=true` e uma lista explícita de `Host` permitidos.

Quando `GET /api/printer` retorna HTTP `409`, `printer_get_status` mantém os dados de conexão e omite `printer`. Outros erros do OctoPrint são retornados como erro seguro da ferramenta.

O transporte é HTTP em texto claro. Em desenvolvimento, ele se limita ao loopback. Uso fora do loopback só é aceitável em rede privada ou VPN, com `Host` e `Origin` explícitos e o token protegido; esse cenário não foi implantado nem validado. Não há suporte a TLS neste incremento.

## Desenvolvimento

Comandos verificados para o estado atual do projeto:

```sh
go test ./...
go test -race ./...
go vet ./...
```

## Limites do escopo

O serviço remoto se limita à leitura de conexão e estado da impressora, temperaturas, trabalho atual e arquivos disponíveis. Não deve enviar arquivos, iniciar ou controlar impressões, definir temperaturas, mover eixos ou enviar G-code arbitrário. As ferramentas MCP não expõem argumentos de escrita e o cliente HTTP só emite GET aprovados. CAD e fatiamento são executados localmente em um computador de desenvolvimento, em um incremento separado.

Consulte a documentação canônica de produto e técnica na wiki do workspace.
