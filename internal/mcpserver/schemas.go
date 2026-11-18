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
    "files": {"type": "array", "items": {"$ref": "#/$defs/file"}},
    "free": {"type": "integer", "description": "Free bytes on the local uploads filesystem"},
    "total": {"type": "integer", "description": "Total bytes on the local uploads filesystem"}
  },
  "required": ["files"],
  "additionalProperties": false,
  "$defs": {` + fileSchemaDefinition + `}
}`)

var uploadOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "file": {"$ref": "#/$defs/file"},
    "done": {"type": "boolean"},
    "effectiveSelect": {"type": "boolean"},
    "effectivePrint": {"type": "boolean"}
  },
  "additionalProperties": false,
  "$defs": {` + fileSchemaDefinition + `}
}`)

const temperatureSchemaDefinition = `"temperature": {
  "type": "object",
  "properties": {
    "actual": {"type": "number"},
    "target": {"type": "number"},
    "offset": {"type": "number"}
  },
  "additionalProperties": false
}`

var versionOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "api": {"type": "string"},
    "server": {"type": "string"},
    "text": {"type": "string"}
  },
  "additionalProperties": false
}`)

var serverOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "version": {"type": "string"},
    "safemode": {"oneOf": [{"type": "string"}, {"type": "boolean", "const": false}]}
  },
  "additionalProperties": false
}`)

var toolTemperaturesOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "tools": {
      "type": "object",
      "additionalProperties": {"$ref": "#/$defs/temperature"}
    }
  },
  "required": ["tools"],
  "additionalProperties": false,
  "$defs": {` + temperatureSchemaDefinition + `}
}`)

var bedTemperatureOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "bed": {"$ref": "#/$defs/temperature"}
  },
  "additionalProperties": false,
  "$defs": {` + temperatureSchemaDefinition + `}
}`)

var chamberTemperatureOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "chamber": {"$ref": "#/$defs/temperature"}
  },
  "additionalProperties": false,
  "$defs": {` + temperatureSchemaDefinition + `}
}`)

var sdStateOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "ready": {"type": "boolean"}
  },
  "required": ["ready"],
  "additionalProperties": false
}`)

var printerErrorOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "consequence": {"type": "string"},
    "error": {"type": "string"},
    "faq": {"type": "string"},
    "reason": {"type": "string"}
  },
  "additionalProperties": false
}`)

const profileSchemaDefinitions = `"profile": {
  "type": "object",
  "properties": {
    "id": {"type": "string"},
    "name": {"type": "string"},
    "color": {"type": "string"},
    "model": {"type": "string"},
    "default": {"type": "boolean"},
    "current": {"type": "boolean"},
    "heatedBed": {"type": "boolean"},
    "heatedChamber": {"type": "boolean"},
    "volume": {"$ref": "#/$defs/volume"},
    "extruder": {"$ref": "#/$defs/extruder"}
  },
  "additionalProperties": false
},
"volume": {
  "type": "object",
  "properties": {
    "width": {"type": "number"},
    "depth": {"type": "number"},
    "height": {"type": "number"},
    "formFactor": {"type": "string"},
    "origin": {"type": "string"}
  },
  "additionalProperties": false
},
"extruder": {
  "type": "object",
  "properties": {
    "count": {"type": "integer"},
    "nozzleDiameter": {"type": "number"},
    "sharedNozzle": {"type": "boolean"}
  },
  "additionalProperties": false
}`

var profilesOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "profiles": {
      "type": "object",
      "additionalProperties": {"$ref": "#/$defs/profile"}
    }
  },
  "required": ["profiles"],
  "additionalProperties": false,
  "$defs": {` + profileSchemaDefinitions + `}
}`)

var slicingOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "profiles": {
      "type": "object",
      "additionalProperties": {"type": "array", "items": {"type": "string"}}
    }
  },
  "required": ["profiles"],
  "additionalProperties": false
}`)

var timelapseOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "config": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["off", "zchange", "timed"]},
        "postRoll": {"type": "integer"},
        "fps": {"type": "integer"},
        "retractionZHop": {"type": "number"},
        "minDelay": {"type": "integer"},
        "interval": {"type": "integer"},
        "renderAfterPrint": {"type": "string"}
      },
      "additionalProperties": false
    },
    "files": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "size": {"type": "string"},
          "bytes": {"type": "integer"},
          "date": {"type": "string"}
        },
        "additionalProperties": false
      }
    },
    "unrendered": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "size": {"type": "string"},
          "bytes": {"type": "integer"},
          "date": {"type": "string"},
          "recording": {"type": "boolean"},
          "rendering": {"type": "boolean"},
          "processing": {"type": "boolean"}
        },
        "additionalProperties": false
      }
    }
  },
  "required": ["config", "files", "unrendered"],
  "additionalProperties": false
}`)

var settingsOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "feature": {
      "type": "object",
      "properties": {
        "temperatureGraph": {"type": "boolean"},
        "sdSupport": {"type": "boolean"},
        "modelSizeDetection": {"type": "boolean"},
        "printStartConfirmation": {"type": "boolean"},
        "printCancelConfirmation": {"type": "boolean"},
        "uploadOverwriteConfirmation": {"type": "boolean"},
        "fileDeleteConfirmation": {"type": "boolean"}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`)
