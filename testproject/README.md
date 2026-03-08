# testproject

A minimal Go monorepo used as a fixture for `pkg/analyzer` integration tests
and as the target project for the BuildGraph CI workflow.

## Structure

```
testproject/
├── buildgraph.yaml        # BuildGraph config — scans services/
├── go.mod                 # module: github.com/bubunyo/buildgraph/testproject
├── core/
│   ├── module-a/          # Shared library used by both services
│   │   └── foo.go         # Process(), Fetch(), Transform()
│   └── module-b/          # Shared library used only by service-a
│       └── bar.go         # Save(), Delete()
└── services/
    ├── service-a/         # Depends on module-a and module-b
    │   └── main.go        # Calls module_a.Process → module_b.Save
    └── service-b/         # Depends on module-a only
        └── main.go        # Calls module_a.Fetch, module_a.Transform
```

## Dependency graph

```
service-a → module-a (Process, via Transform)
service-a → module-b (Save)
service-b → module-a (Fetch, Transform)
```

Because `module-a` is shared, a change there will cause **both** services to
be flagged for rebuild. A change to `module-b` only affects `service-a`.
`service-b` is isolated from `module-b` entirely.

## What tests it

The `pkg/analyzer` integration tests load this project via `go/packages` to
exercise the full analysis pipeline — call graph construction, hash
computation, and source hash caching — against real Go source.
