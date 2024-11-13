# Updating Documentation

All documentation for `etcd-druid` resides in [docs](https://github.com/gardener/etcd-druid/tree/master/docs) directory. If you wish to update the existing documentation or create new documentation files then read on.

## Prerequisite: Setup Mkdocs locally
[Material for Mkdocs](https://squidfunk.github.io/mkdocs-material/) is used to generate GitHub Pages from all the Markdown files present under the [docs](https://github.com/gardener/etcd-druid/tree/master/docs) directory. To locally validate that the documentation renders correctly, it is recommended that you perform the following setup.

* Install python3 if not already installed.
* Setup a virtual environment via 

```bash
python -m venv venv
```

* Activate the virtual environment 

```bash
source venv/bin/activate
```

* In the virtual environment install the packages.

```bash
(venv) > pip install mkdocs-material
(venv) > pip install pymdown-extensions
(venv) > pip install mkdocs-glightbox
(venv) > pip install mkdocs-pymdownx-material-extras
```

!!! note
    Complete list of packages installed should be in sync with [Github Actions Configuration](https://github.com/gardener/etcd-druid/blob/master/.github/workflows/publish-docs.yml#L23-L27).
* Serve the documentation

```bash
(venv) > mkdocs serve
```
You can now view the rendered documentation at `localhost:8000`. Any changes that you make to the docs will get hot-reloaded and you can immediately view the changes.

## Updating Documentation

All documentation _should_ be in `markdown` _only_. Ensure that you take care of the following:

* The [index.md](https://github.com/gardener/etcd-druid/blob/master/docs/index.md) is the home page for the documentation rendered as Github Pages. Please do not remove this file.
* If you are using a new feature (that is not already used) by `Mkdocs` then ensure that it is properly configured in [mkdocs.yml](https://github.com/gardener/etcd-druid/blob/master/mkdocs.yml). Additionally, if new plugins or Markdown extensions are used, make sure that you update the [Github Actions Configuration](https://github.com/gardener/etcd-druid/blob/master/.github/workflows/publish-docs.yml#L23-L27) accordingly.
* If new files are being added and you wish to show these files in Github Pages then ensure that you have added them under appropriate sections in the [navigation](https://github.com/gardener/etcd-druid/blob/master/mkdocs.yml#L70) section of `mkdocs.yml`.
* If you are linking to any file outside the [docs](https://github.com/gardener/etcd-druid/tree/master/docs) directory then relative links will not work on Github Pages. Please get the `https` link to the file or section of the file that you wish to link.

## Raise a Pull Request

Once you have made the documentation changes then follow the guide on [how to raise a PR](raising-a-pr.md).

!!! info
    Once the documentation update PR has been merged, you will be able to see the updated documentation [here](https://gardener.github.io/etcd-druid/).


