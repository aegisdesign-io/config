[![Tests](https://github.com/aegisdesign-io/config/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/aegisdesign-io/config/actions/workflows/test.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/aegisdesign-io/config/badges/coverage.json)](https://github.com/aegisdesign-io/config/actions/workflows/test.yml)

# config

A lightweight application configuration library that uses JSON, YAML, and 
environment variable overrides.

## Features

* File formats: JSON and YAML
* Environment variables override values read from file.
* Strongly typed configuration data (no key-value maps)
* Data structure is defined by the application.

## Usage

A common use-case for configuration files is for microservice applications.
As an example, let's say we need to configure the port number the web service
listens on and the connection information for a database. The YAML file could
look like this:

```YAML
---
server:
  port: 8080

database:
  driver:   postgres
  hostname: localhost
  port:     5432
  username: postgres
  password: dummy
  name:     my_database
```

Now in the Go application a data structure is defined to match:

```go
type AppConfig struct {
    Server struct {
        Port uint16
    }
    Database struct {
        Driver   string
        Hostname string
        Port     uint16
        Username string
        Password string
        Name     string
    }
}
```

To ingest the configuration file into the application is two lines of code:

```go
var c = new(AppConfig)
var err = config.FromFile("/path/of/config.yaml", c)
```

### Field names and struct tags

JSON files are decoded with `encoding/json` and YAML files with
[`go.yaml.in/yaml/v4`](https://pkg.go.dev/go.yaml.in/yaml/v4). Each library
reads only its own struct tags:

* **JSON** uses `json:"..."` tags. An untagged field matches a key with the same
  name, ignoring case.
* **YAML** uses `yaml:"..."` tags. An untagged field matches only its **lowercased**
  name, so `Hostname` matches `hostname` but not `hostName`. YAML ignores `json`
  tags.
* **Embedded structs** are flattened by JSON automatically. YAML flattens them
  only when they are tagged `yaml:",inline"`.

The example above needs no tags because every key is the lowercased field name.
If a struct is loaded from both formats, or its keys differ from the field
names, tag both:

```go
type Database struct {
    Hostname string `json:"host" yaml:"host"`
}

type Config struct {
    Common `yaml:",inline"`
    DB Database `json:"database" yaml:"database"`
}
```

### Environment variable overrides

Tag a field with `env:"NAME"` and, if `NAME` is set, its value replaces
whatever was read from the file:

```go
type AppConfig struct {
    Server struct {
        Port    uint16        `env:"SERVER_PORT"`
        Timeout time.Duration `env:"SERVER_TIMEOUT"` // e.g. "30s"
    }
    Database struct {
        Password string `env:"DB_PASSWORD"`
    }
}
```

* Overrides are applied after the file is decoded.
* Supported types: strings, booleans, integers, unsigned integers, floats,
  pointers to these, `time.Duration` (as an integer of nanoseconds or a
  duration string like `30s`), and any type implementing
  `encoding.TextUnmarshaler`.
* An empty value overrides string fields only; for other types it is treated
  as unset.
* Nested structs and pointers to structs are searched. A nil pointer is only
  allocated if a variable inside it is set.
* Fields must be exported. Setting a variable that targets an unexported field
  returns an error.

## Contributing

 1.  Fork it
 2.  Create a feature branch (`git checkout -b new-feature`)
 3.  Commit changes (`git commit -am "Added new feature xyz"`)
 4.  Push the branch (`git push origin new-feature`)
 5.  Create a new pull request.

## Maintainers

* [Aegis Design](http://github.com/aegisdesign-io/)

## License

   Copyright 2026 AegisDesign.io

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
