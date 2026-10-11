Run with:

```bash
go run ./webalator --listen=127.0.0.1:8080 --debug-listen=127.0.0.1:8081 --logtostderr=true
```

Site content lives in `sitecontent/static` and `sitecontent/templates`, and is
embedded into the binary.
