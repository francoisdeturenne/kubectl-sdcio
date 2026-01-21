# kubectl-sdcio

![sdc logo](https://docs.sdcio.dev/assets/logos/SDC-transparent-withname-100x133.png)

kubectl-sdcio is the SDC specific kubectl plugin.

## subcommands

kubectl-sdcio provides the following functionalities.

### blame

The blame command provides a tree based view on the actual running device configuration of the given SDC target.

It takes the `--target` parameter, that defines which targets is to be displayed.

For every configured attribute you will see the highes preference value as well
as the source of that value.

- `running` are attributes that come from the device itself, where no intent exist in sdcio.
- `default` is all the default values that are present in the config, that are not overwritten by any specific config.
- `<namespace>.<intentname>` is the reference to the intent that defined the actual highes preference value for that config attribute.

```bash
mava@server01:~/projects/kubectl-sdcio$ kubectl sdcio blame --target sros
                    -----    │     🎯 default.sros
                    -----    │     └── 📦 configure
                    -----    │         ├── 📦 card
                    -----    │         │   └── 📦 1
                  default    │         │       ├── 🍃 admin-state -> enable
                  running    │         │       ├── 🍃 card-type -> iom-1
                  default    │         │       ├── 🍃 fail-on-error -> false
                  default    │         │       ├── 🍃 filter-profile -> none
                  default    │         │       ├── 🍃 hash-seed-shift -> 2
                  default    │         │       ├── 🍃 power-save -> false
                  default    │         │       ├── 🍃 reset-on-recoverable-error -> false
                  running    │         │       └── 🍃 slot-number -> 1
                    -----    │         ├── 📦 service
                    -----    │         │   ├── 📦 customer
                    -----    │         │   │   ├── 📦 1
    default.customer-sros    │         │   │   │   ├── 🍃 customer-id -> 1
    default.customer-sros    │         │   │   │   └── 🍃 customer-name -> 1
                    -----    │         │   │   └── 📦 2
    default.customer-sros    │         │   │       ├── 🍃 customer-id -> 2
    default.customer-sros    │         │   │       └── 🍃 customer-name -> 2
                    -----    │         │   └── 📦 vprn
                    -----    │         │       ├── 📦 vprn123
default.intent1-sros-sros    │         │       │   ├── 🍃 admin-state -> enable
                  default    │         │       │   ├── 🍃 allow-export-bgp-vpn -> false
                  default    │         │       │   ├── 🍃 carrier-carrier-vpn -> false
                  default    │         │       │   ├── 🍃 class-forwarding -> false
default.intent1-sros-sros    │         │       │   ├── 🍃 customer -> 1
...
```

#### Filtering Options

The blame command supports several filtering options to narrow down the results. **All filters are cumulative** (combined with "AND" logic), meaning only configuration elements that match ALL specified criteria will be displayed.

Available filters:

- **`--filter-leaf <pattern>`**: Filter by leaf node name. Supports wildcards (`*`).
  - Example: `--filter-leaf "admin-state"` shows only admin-state leaves
  - Example: `--filter-leaf "interface*"` shows all leaves starting with "interface"

- **`--filter-owner <pattern>`**: Filter by configuration owner. Supports wildcards (`*`).
  - Example: `--filter-owner "running"` shows only running configuration
  - Example: `--filter-owner "default.*"` shows all default configurations
  - Example: `--filter-owner "production.intent-*"` shows intents from production namespace

- **`--filter-path <pattern>`**: Filter by configuration path. Supports wildcards (`*`).
  - Example: `--filter-path "/config/service/*"` shows only service-related configuration
  - Example: `--filter-path "*/interface/*"` shows interface configuration at any level
The whole path (including leaves) is involved in the pattern matching.

- **`--filter-deviation`**: Show only configuration elements that have deviations between intended and actual values.

#### Filter Examples

```bash
# Show only admin-state configuration from running config
kubectl sdcio blame --target sros --filter-leaf "admin-state" --filter-owner "running"

# Show all interface-related configuration with deviations
kubectl sdcio blame --target sros --filter-path "*/interface/*" --deviation

# Show configuration from specific intent with timeout-related leaves
kubectl sdcio blame --target sros --filter-owner "production.intent-emergency" --filter-leaf "*timeout*"

# Combine multiple filters to find specific configuration
kubectl sdcio blame --target sros --filter-path "/config/service/emergency/*" --filter-leaf "ambulance" --filter-owner "test-system.*"

## search-for

The search-for command searches for keywords in YANG models and returns matching paths. This is useful for discovering configuration paths and understanding the structure of YANG models. Outputs from the `search-for` command may be used as input of the `--path` option of the `gen` command.

Usage

```bash

kubectl sdcio search-for --yang <yang-file> --yang-search <keyword> [options]
```

#### Required Parameters

- **`--yang <path>`** : Path to the YANG module file (required)
- **`--yang-search <keyword>`** : Keyword to search for. Supports wildcards (*) (required)

#### Optional Parameters

- **`--format <format>`**: Output format: text, json, or yaml (default: text)
- **`--output <file> / -o <file>`**: Output file path (default: stdout)
- **`--case-sensitive`**: Enable case-sensitive search (default: false)
- **`--deepy`**: Include dependency information in results (default: true)

### Wildcards

The search supports wildcard patterns:

- **`*`** : - matches any sequence of characters

Examples

```bash

# Search for exact leaf name
kubectl sdcio search-for --yang model.yang --yang-search ambulance

# Search with wildcard
kubectl sdcio search-for --yang model.yang --yang-search "*timeout*"

# Search case-sensitive
kubectl sdcio search-for --yang model.yang --yang-search "Interface" --case-sensitive

# Output as JSON
kubectl sdcio search-for --yang model.yang --yang-search "*config*" --format json

# Save results to file
kubectl sdcio search-for --yang model.yang --yang-search "*ip*" --output results.txt

# Search without dependency information
kubectl sdcio search-for --yang model.yang --yang-search "interface" --deepy=false
```

### Output Format

The command returns matching paths with the following information:

- Path: The full YANG path to the matching element
- Leaf Name: The name of the leaf/node
- Type: The YANG type (container, leaf, list, etc.)
- Keys: List keys (if applicable)
- Description: YANG description (if available)
- Dependencies: Dependency information including leafrefs, when conditions, must statements, and reverse references (when --deepy is enabled)

## gen

The gen command generates configuration templates from YANG models in various
formats. This is useful for creating initial configuration files or
understanding the structure of configuration data.

### Usage

```bash

kubectl sdcio gen --yang <yang-file> [options]
```

#### Required Parameters

- **`--yang <path>`** : Path to the YANG module file (required)

#### Optional Parameters

- **`--path <path>`**: Path in the YANG model to generate template for (default: / - root)
- **`--format <format>`**: Output format: json, yaml, xml, or sdc-conf (default: json)
- **`--output <file> / -o <file>`**: Output file path (default: stdout)

Examples

```bash

# Generate JSON template for entire model
kubectl sdcio gen --yang model.yang

# Generate template for specific path
kubectl sdcio gen --yang openconfig-interfaces.yang --path /interfaces/interface

# Generate YAML template and save to file
kubectl sdcio gen --yang model.yang --path /system/config --format yaml --output config.yaml

# Generate XML template
kubectl sdcio gen --yang model.yang --path /configure/service --format xml

# Generate SDC Config custom resource
kubectl sdcio gen --yang model.yang --path /interfaces --format sdc-conf --output intent.yaml
```

### Template Features

The generated templates include:

- Example values based on YANG types (strings, integers, booleans, etc.)
- Metadata for lists (keys, min/max elements)
- Type information and constraints (ranges, patterns)
- Proper structure for containers, lists, and leaf-lists
- Key placeholders for list entries
- Namespace information (for XML format)

### SDC Config Format

When using --format sdc-conf, the command generates a complete SDC Config custom resource with:

- API version and kind
- Metadata with name and namespace
- Lifecycle configuration
- Priority and revertive settings
- Config section with the specified path and generated template

## Join us

Have questions, ideas, bug reports or just want to chat? Come join [our discord server](https://discord.com/channels/1240272304294985800/1311031796372344894).

## License and Code of Conduct

Code is under the [Apache License 2.0](LICENSE), documentation is [CC BY 4.0](LICENSE-documentation).

The SDC project is following the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). More information and links about the CNCF Code of Conduct are [here](https://www.cncf.io/conduct/).
