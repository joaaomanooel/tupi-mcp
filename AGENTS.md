# Project rules

- Keep the OctoPrint integration read-only. The client may issue only explicitly allowed HTTP GET requests.
- Do not add upload, print, pause, cancel, temperature, motion, or arbitrary G-code operations.
- Never store credentials in Git or include them in logs, errors, test output, or tool responses.
- Use mock HTTP servers for development. Do not connect to or modify the Raspberry Pi or physical printer unless the user separately authorizes that specific action.
- Do not claim Codex or OpenCode compatibility until each real client has been tested.
- Treat CAD and slicing as a separate local-workstation increment. Do not run them on the Raspberry Pi.
- Keep project code and identifiers in English and primary documentation in Brazilian Portuguese.
