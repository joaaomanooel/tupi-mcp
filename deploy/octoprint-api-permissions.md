# Permissões das ferramentas do Tupi MCP

O Tupi MCP usa as permissões do usuário OctoPrint associado à API key mantida
pelo servidor. Para habilitar as ferramentas desta versão, atribua ao usuário
dedicado as permissões da tabela em um grupo próprio.

| Permissão no OctoPrint 1.11.8 | Ferramentas MCP |
| --- | --- |
| `STATUS` | `printer_get_status`, `printer_get_temperatures`, `printer_get_job`, `printer_get_version`, `printer_get_server`, `printer_get_tool_temperatures`, `printer_get_bed_temperature`, `printer_get_chamber_temperature`, `printer_get_sd_state`, `printer_get_last_error` |
| `FILES_LIST` | `printer_list_files` |
| `FILES_UPLOAD` | `printer_upload_gcode`, `printer_create_folder`, `printer_copy_file`, `printer_move_file` |
| `FILES_DELETE` | `printer_delete_file`, `printer_move_file` |
| `FILES_SELECT` | `printer_select_file` |
| `SLICE` | `printer_get_slicing`, `printer_slice_file` |
| `PRINT` | `printer_start_job`, `printer_pause_job`, `printer_resume_job`, `printer_restart_job`, `printer_cancel_job` |
| `CONNECTION` | `printer_get_profiles`, `printer_connect`, `printer_reconnect`, `printer_disconnect` |
| `CONTROL` | `printer_jog`, `printer_home`, `printer_set_feedrate`, `printer_set_tool_temperature`, `printer_set_tool_offset`, `printer_select_tool`, `printer_extrude`, `printer_set_bed_temperature`, `printer_set_bed_offset`, `printer_set_chamber_temperature`, `printer_set_chamber_offset`, `printer_init_sd`, `printer_refresh_sd`, `printer_release_sd` |
| `TIMELAPSE_LIST` | `printer_get_timelapse` |
| `SETTINGS_READ` | `printer_get_settings` |

`CONNECTION` também autoriza conectar e desconectar pela API do OctoPrint.
`CONTROL` também autoriza movimento, aquecimento, seleção de ferramenta,
extrusão/retração e operações SD. `SLICE` autoriza fatiar arquivos pela API.
`FILES_UPLOAD` autoriza criar pastas e copiar arquivos; junto com
`FILES_DELETE`, também autoriza mover arquivos. `FILES_DELETE` autoriza excluir
arquivos. `PRINT` autoriza operações de trabalho pela API. As ferramentas MCP
oferecem operações mais restritas. Proteja a API key conforme as permissões do
usuário. [Definições de permissões do OctoPrint 1.11.8](https://github.com/OctoPrint/OctoPrint/blob/1.11.8/src/octoprint/access/permissions.py)

## Exposição das ferramentas de arquivos e jobs

As ferramentas `printer_create_folder`, `printer_copy_file`,
`printer_move_file`, `printer_select_file`, `printer_delete_file`,
`printer_slice_file`, `printer_start_job`, `printer_pause_job`,
`printer_resume_job`, `printer_restart_job` e `printer_cancel_job` ficam
desativadas por padrão. Habilite somente as operações autorizadas em
`TUPI_MCP_ENABLED_TOOLS`, com nomes exatos separados por vírgula. Nomes
desconhecidos ou repetidos impedem a inicialização do servidor.

## Exposição dos controles físicos

As ferramentas físicas `printer_connect`, `printer_reconnect`,
`printer_disconnect`, `printer_jog`, `printer_home`, `printer_set_feedrate`,
`printer_set_tool_temperature`, `printer_set_tool_offset`, `printer_select_tool`,
`printer_extrude`, `printer_set_bed_temperature`, `printer_set_bed_offset`,
`printer_set_chamber_temperature`, `printer_set_chamber_offset`,
`printer_init_sd`, `printer_refresh_sd` e `printer_release_sd` também ficam
desativadas por padrão. Habilite cada operação explicitamente em
`TUPI_MCP_ENABLED_TOOLS`. Jog/home, extrusão e operações de aquecedores ou
ferramentas dependem dos limites físicos válidos e do perfil aplicável. Sem
configuração válida, as ferramentas dependentes desses dados não aparecem em
`tools/list`; leituras e ferramentas sem limites numéricos aplicáveis continuam
funcionando.

As operações de conexão exigem `CONNECTION`; os outros controles físicos exigem
`CONTROL`. Os handlers revalidam estado, perfil, limites e precondições antes
de cada escrita. Uma resposta `accepted` confirma somente que o OctoPrint aceitou
a requisição; não confirma movimento, aquecimento ou execução física. Uma
resposta `unknown` indica que o resultado não pôde ser confirmado e a operação
não é repetida automaticamente. Não existe ferramenta MCP para executar G-code
arbitrário ou escolher um path ou endpoint arbitrário.

## Transporte e identidade compartilhada

O transporte agente→MCP não exige bearer token nem outra credencial. Os guards
existentes de `TUPI_MCP_ALLOWED_HOSTS` e `TUPI_MCP_ALLOWED_ORIGINS` continuam
limitando requisições conforme configuração, mas não autenticam clientes nem
separam permissões por cliente. Todo cliente que alcança o endpoint compartilha
a allowlist habilitada e a identidade OctoPrint associada à API key do servidor.
Restrinja a exposição de rede conforme o ambiente de implantação. Não coloque
a API key em configuração versionada, instruções MCP, logs ou mensagens de
erro.
