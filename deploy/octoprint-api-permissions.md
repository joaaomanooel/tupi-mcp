# Permissões das ferramentas do Tupi MCP

O Tupi MCP usa as permissões do usuário OctoPrint associado à chave de aplicação. Para habilitar as ferramentas desta versão, atribua ao usuário dedicado as permissões da tabela em um grupo próprio.

| Permissão no OctoPrint 1.11.8 | Ferramentas MCP |
| --- | --- |
| `STATUS` | `printer_get_status`, `printer_get_temperatures`, `printer_get_job`, `printer_get_version`, `printer_get_server`, `printer_get_tool_temperatures`, `printer_get_bed_temperature`, `printer_get_chamber_temperature`, `printer_get_sd_state`, `printer_get_last_error` |
| `FILES_LIST` | `printer_list_files` |
| `FILES_UPLOAD` | `printer_upload_gcode`, `printer_create_folder`, `printer_copy_file`, `printer_move_file` |
| `FILES_DELETE` | `printer_delete_file`, `printer_move_file` |
| `FILES_SELECT` | `printer_select_file` |
| `SLICE` | `printer_get_slicing`, `printer_slice_file` |
| `PRINT` | `printer_start_job`, `printer_pause_job`, `printer_resume_job`, `printer_restart_job`, `printer_cancel_job` |
| `CONNECTION` | `printer_get_profiles` |
| `TIMELAPSE_LIST` | `printer_get_timelapse` |
| `SETTINGS_READ` | `printer_get_settings` |

`CONNECTION` também autoriza conectar e desconectar pela API do OctoPrint. `SLICE` também autoriza fatiar arquivos pela API. `FILES_UPLOAD` também autoriza criar pastas e copiar arquivos pela API e, junto com `FILES_DELETE`, mover arquivos. `FILES_DELETE` também autoriza excluir arquivos e, junto com `FILES_UPLOAD`, movê-los. `PRINT` também autoriza imprimir pela API. As ferramentas MCP oferecem operações mais restritas. Proteja a chave de aplicação conforme as permissões do usuário. [Definições de permissões do OctoPrint 1.11.8](https://github.com/OctoPrint/OctoPrint/blob/1.11.8/src/octoprint/access/permissions.py)

## Exposição das ferramentas de arquivos e jobs

As ferramentas `printer_create_folder`, `printer_copy_file`, `printer_move_file`, `printer_select_file`, `printer_delete_file`, `printer_slice_file`, `printer_start_job`, `printer_pause_job`, `printer_resume_job`, `printer_restart_job` e `printer_cancel_job` ficam desativadas por padrão. A exposição é explícita: habilite somente as operações autorizadas em `TUPI_MCP_ENABLED_TOOLS`, com nomes exatos separados por vírgula. Nomes desconhecidos ou repetidos impedem a inicialização do servidor.

O transporte MCP não usa bearer token. Qualquer cliente aceito por `TUPI_MCP_ALLOWED_HOSTS` e `TUPI_MCP_ALLOWED_ORIGINS` pode chamar as ferramentas habilitadas, então restrinja a exposição de rede ao cliente pretendido. O upload existente continua disponível e exige `FILES_UPLOAD`.
