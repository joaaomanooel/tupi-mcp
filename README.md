# Tupi MCP

O Tupi MCP, no repositório `tupi-mcp`, é um serviço em Go em desenvolvimento para expor informações do OctoPrint em modo somente leitura a clientes MCP.

## Estado atual

O cliente HTTP interno de leitura do OctoPrint e o mapeamento de suas respostas estão implementados. Seus quatro métodos fazem requisições GET aos endpoints aprovados, validam JSON e os limites de resposta e protegem a chave de API. Eles retornam tipos internos para conexão, estado da impressora e temperaturas disponíveis, trabalho atual e progresso, estimativas e lista de arquivos. Campos opcionais ausentes permanecem ausentes; respostas sem trabalho atual e listas vazias são válidas. Campos desconhecidos são ignorados, e tipos incompatíveis em campos conhecidos retornam erros seguros.

Ainda não estão implementados: ferramentas e transporte MCP, autenticação e autorização MCP, inicialização e carregamento da configuração, serviço mock, implantação e validação com hardware. Os tipos internos não definem os esquemas públicos das ferramentas MCP. A licença do projeto permanece pendente. O MVP não está concluído.

As verificações automatizadas atuais usam mocks locais com `httptest`. Esses resultados não comprovam compatibilidade com uma instância real do OctoPrint nem com Codex ou OpenCode.

## Desenvolvimento

Comandos verificados para o estado atual do projeto:

```sh
go test ./...
go test -race ./...
go vet ./...
```

## Limites do escopo

O serviço remoto se limita à leitura de conexão e estado da impressora, temperaturas, trabalho atual e arquivos disponíveis. Não deve enviar arquivos, iniciar ou controlar impressões, definir temperaturas, mover eixos ou enviar G-code arbitrário. CAD e fatiamento são executados localmente em um computador de desenvolvimento, em um incremento separado.

Consulte a documentação canônica de produto e técnica na wiki do workspace.
