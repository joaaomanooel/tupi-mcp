# Permissões das ferramentas do Tupi MCP

O Tupi MCP usa as permissões do usuário OctoPrint associado à chave de aplicação. Para habilitar as ferramentas desta versão, atribua ao usuário dedicado as permissões da tabela em um grupo próprio.

| Permissão no OctoPrint 1.11.8 | Ferramentas MCP |
| --- | --- |
| `STATUS` | Estado da conexão e da impressora, temperaturas, job, versão, servidor, estado do SD e último erro |
| `FILES_LIST` | Lista de arquivos e capacidade do volume local de uploads |
| `FILES_UPLOAD` | Upload local de G-code |
| `CONNECTION` | Lista de perfis da impressora |
| `SLICE` | Lista de perfis de fatiamento |
| `TIMELAPSE_LIST` | Configuração e lista de timelapses renderizados e pendentes |
| `SETTINGS_READ` | Leitura de sete opções de recurso em `printer_get_settings` |

`CONNECTION` também autoriza conectar e desconectar pela API do OctoPrint. `SLICE` também autoriza fatiar arquivos pela API. `FILES_UPLOAD` também autoriza criar pastas e copiar arquivos pela API. As ferramentas MCP oferecem operações mais restritas. Proteja a chave de aplicação conforme as permissões do usuário. [Definições de permissões do OctoPrint 1.11.8](https://github.com/OctoPrint/OctoPrint/blob/1.11.8/src/octoprint/access/permissions.py)
