# Estação de trabalho 3D: OpenSCAD e OrcaSlicer

Guia da estação local que prepara o primeiro trabalho de impressão do projeto. O OpenSCAD 2026.10.11 serve para visualizar a geometria, e o OrcaSlicer 2.5.1 fatia o STL bruto com o perfil da Creality Ender-3 V3 SE. Nada aqui envia arquivos ou comandos ao OctoPrint: a integração remota continua somente leitura.

## Escopo deste incremento

- O trabalho de estreia é o modelo 225996 do MakerWorld, perfil `1100291`: um raspador de 86 mm de comprimento, 5 mm de espessura e cerca de 3 a 4 g.
- A página de origem está em https://makerworld.com/pt/models/225996-mini-bed-scraper#profileId-1100291. Leia a descrição e as observações do autor antes de fatiar.
- A página oferece perfis para máquinas Bambu, incluindo 0,08 mm de camada, 2 paredes e 15% de preenchimento. Esses números são referência apenas: a Ender-3 V3 SE tem mesa, hotend e cinemática diferentes.
- Use sempre o STL bruto com o perfil da Ender-3 V3 SE. Não baixe nem carregue G-code daquele perfil, e não envie G-code ao OctoPrint nesta etapa.
- O modelo, o perfil e o G-code não são copiados para o repositório. Só o link e os dados de referência ficam registrados aqui.

## Sequência offline

1. Baixe apenas o arquivo STL bruto do modelo em uma máquina com acesso à internet. Não baixe o `.gcode.3mf` do perfil Bambu.
2. Copie o STL para a estação de trabalho por pendrive ou rede local. A partir daqui o fluxo não precisa de internet.
3. Abra o STL no OpenSCAD 2026.10.11. O arquivo entra no editor como `import("caminho/do/arquivo.stl");`; pressione `F5` para renderizar e use o mouse para girar e aproximar.
4. Use o OpenSCAD para conferir visualmente a forma e a escala; a visualização não substitui a validação ou o reparo da malha. O STL não carrega unidade: considere 1 unidade como 1 mm.
5. Abra o OrcaSlicer 2.5.1 e selecione a impressora `Creality Ender-3 V3 SE` com bico de 0,4 mm. Se o perfil não aparecer na lista de impressoras do sistema, registre a ausência em vez de criar um perfil genérico de outra máquina.
6. Importe o STL bruto com `Arquivo > Importar > Importar STL` ou arrastando o arquivo para a mesa.
7. Confira orientação e estabilidade sobre a mesa conforme a indicação do autor na página do modelo, e verifique se a peça cabe no volume de 220 × 220 × 250 mm da Ender-3 V3 SE. Use a ferramenta de medição do OrcaSlicer para confirmar os 86 mm de comprimento e os 5 mm de espessura.
8. Parta do perfil de processo da Ender-3 V3 SE e só depois mude o que for necessário. Os valores da página do modelo servem de comparação, não de prescrição; 0,08 mm de camada, por exemplo, reduz a altura das camadas e aumenta bastante o tempo de impressão.
9. Selecione PLA preto de 1,75 mm no OrcaSlicer. A embalagem recomenda bico entre 200 e 230 °C e mesa entre 45 e 65 °C; confirme que o perfil escolhido fica dentro dessas faixas antes de fatiar. O carretel informa peso líquido de 1 kg.
10. Fatie com `Fatiar`, revise a pré-visualização camada por camada e confira tempo estimado e peso. O intervalo de 3 a 4 g do modelo é apenas uma expectativa de conferência.
11. Exporte o G-code para um arquivo local, como um cartão SD ou pendrive. Não envie o arquivo ao OctoPrint e não inicie impressão a partir dele.

## Referência de fatiamento

| Item | Valor | Como usar |
| --- | --- | --- |
| Comprimento | 86 mm | Conferir na medição do OrcaSlicer |
| Espessura | 5 mm | Conferir na medição do OrcaSlicer |
| Peso esperado | 3 a 4 g | Comparar com a estimativa após fatiar |
| Altura de camada | 0,08 mm | Referência de perfil Bambu; ajustar conforme o perfil da Ender |
| Paredes | 2 | Referência de perfil Bambu; ajustar conforme a resistência desejada |
| Preenchimento | 15% | Referência de perfil Bambu; ajustar conforme a resistência desejada |
| Material | PLA preto, 1,75 mm | Usar um perfil PLA compatível com a Ender-3 V3 SE |
| Temperatura do bico | 200 a 230 °C | Faixa recomendada na embalagem |
| Temperatura da mesa | 45 a 65 °C | Faixa recomendada na embalagem |
| Peso líquido do carretel | 1 kg | Informação da embalagem; não altera o perfil de fatiamento |

## Checklist antes de sair da estação

- [ ] STL bruto importado, sem G-code de outra impressora.
- [ ] Impressora selecionada como Ender-3 V3 SE, bico de 0,4 mm.
- [ ] Peça apoiada na mesa e dentro do volume útil.
- [ ] Perfil PLA preto de 1,75 mm, com bico entre 200 e 230 °C e mesa entre 45 e 65 °C.
- [ ] Pré-visualização revisada e G-code exportado para arquivo local.
- [ ] Nenhum arquivo enviado ao OctoPrint.

## Limites

O OctoPrint permanece um serviço somente leitura para o Tupi MCP. Este guia não cobre envio de arquivos, início de impressão, controle de temperatura ou movimento de eixos, e nada disso deve ser adicionado à integração sem uma decisão separada.
