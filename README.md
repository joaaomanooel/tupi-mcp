# MCP OctoPrint

A small Go service intended to expose read-only OctoPrint information to MCP clients. Initial development uses a deterministic HTTP mock; Raspberry Pi deployment and physical printer validation are later steps.

## Status

Implementation has not started. Transport authentication, final MCP tool schemas, resource limits, project license, and client compatibility remain pending decisions. The first code increment is limited to a read-only OctoPrint client and its mock-based tests.

## Scope boundary

The remote service is limited to reading connection and printer status, temperatures, current job, and available files. It must not upload files, start or control prints, set temperatures, move axes, or send arbitrary G-code. CAD and slicing run locally on a development computer in a separate increment.

See the canonical product and technical documentation in the workspace wiki.
