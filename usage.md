# Watchall Manual

...

```text
watchall [command] [global flags] [command flags]
```

### Global Flags

```text
  -o, --outdir string   Directory to store output (default "watchall-output")
  -v, --verbose         Create more output
```

### Commands

* [watchall deltas](#watchall-deltas)
* [watchall help](#watchall-help)
* [watchall logs](#watchall-logs)
* [watchall record](#watchall-record)

# Commands

## `watchall deltas`

This reads the files from the local disk and shows the changes. No connection to a cluster is needed.

```text
watchall deltas dir [flags]
```

### Command Flags

```text
  -h, --help           help for deltas
      --only strings   comma separated list of regex patterns to show
      --skip strings   comma separated list of regex patterns to skip
```

## `watchall help`

Help about any command

```text
watchall help [command] [flags]
```

### Command Flags

```text
  -h, --help   help for help
```

## `watchall logs`

...

```text
watchall logs [flags]
```

### Command Flags

```text
  -h, --help   help for logs
```

## `watchall record`

...

```text
watchall record [flags]
```

### Command Flags

```text
  -h, --help   help for record
```
