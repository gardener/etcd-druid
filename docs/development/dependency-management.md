# Dependency Management

We use [Go Modules](https://go.dev/wiki/Modules) for dependency management. In order to add a new package dependency to the project, you can perform `go get <package@version>` or edit the `go.mod` file and append the package along with the version you want to use.

## Organize Dependencies

Unfortunately go does not differentiate between `dev` and `test` dependencies. It becomes cleaner to organize `dev` and `test` dependencies in their respective `require` clause which gives a clear view on existing set of dependencies. The goal is to keep the dependencies to a minimum and only add a dependency when absolutely required.

## Updating Dependencies

The `Makefile` contains a rule called `tidy` which performs [go mod tidy](https://go.dev/ref/mod#go-mod-tidy) which ensures that the `go.mod` file matches the source code in the module. It adds any missing module requirements necessary to build the current module’s packages and dependencies, and it removes requirements on modules that don’t provide any relevant packages. It also adds any missing entries to `go.sum` and removes unnecessary entries.

```bash
make tidy
```

!!! warning
    Make sure that you test the code after you have updated the dependencies!
