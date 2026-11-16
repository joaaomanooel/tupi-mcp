package mcpserver

import "encoding/json"

const fileSchemaDefinition = `"file": {
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "display": {"type": "string"},
    "path": {"type": "string"},
    "type": {"type": "string"},
    "origin": {"type": "string"},
    "size": {"type": "integer"},
    "date": {"type": "integer"},
    "children": {"type": "array", "items": {"$ref": "#/$defs/file"}}
  },
  "additionalProperties": false
}`

var statusOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "connection": {
      "type": "object",
      "properties": {
        "state": {"type": "string"},
        "port": {"type": "string"},
        "baudrate": {"type": "integer"},
        "printerProfile": {"type": "string"}
      },
      "additionalProperties": false
    },
    "printer": {
      "type": "object",
      "properties": {
        "state": {
          "type": "object",
          "properties": {
            "text": {"type": "string"},
            "flags": {
              "type": "object",
              "properties": {
                "operational": {"type": "boolean"},
                "printing": {"type": "boolean"},
                "paused": {"type": "boolean"},
                "ready": {"type": "boolean"},
                "error": {"type": "boolean"},
                "closedOrError": {"type": "boolean"}
              },
              "additionalProperties": false
            }
          },
          "additionalProperties": false
        }
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`)

var temperaturesOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "temperatures": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "actual": {"type": "number"},
          "target": {"type": "number"},
          "offset": {"type": "number"}
        },
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false
}`)

var jobOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "job": {
      "type": "object",
      "properties": {
        "file": {"$ref": "#/$defs/file"},
        "estimatedPrintTime": {"type": "number"},
        "averagePrintTime": {"type": "number"},
        "lastPrintTime": {"type": "number"}
      },
      "additionalProperties": false
    },
    "progress": {
      "type": "object",
      "properties": {
        "completion": {"type": "number"},
        "filePosition": {"type": "integer"},
        "printTime": {"type": "number"},
        "printTimeLeft": {"type": "number"},
        "printTimeLeftOrigin": {"type": "string"}
      },
      "additionalProperties": false
    },
    "state": {"type": "string"}
  },
  "additionalProperties": false,
  "$defs": {` + fileSchemaDefinition + `}
}`)

var filesOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "files": {"type": "array", "items": {"$ref": "#/$defs/file"}}
  },
  "required": ["files"],
  "additionalProperties": false,
  "$defs": {` + fileSchemaDefinition + `}
}`)
