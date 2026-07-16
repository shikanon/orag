# Visual-document RAG Recipe

`visual-document-rag/1.0.0` is a reproducible **Recipe**, rather than a
redistributed data Pack. ORAG does not mirror ViDoSeek documents, annotations,
page renders, or derived corpora.

## What Clone does

After you explicitly accept the source licence, Clone resolves only these
immutable upstream inputs into your project-private storage:

| Field | Value |
| --- | --- |
| Dataset | `Qiuchen-Wang/ViDoSeek` |
| Revision | `e91a92ba5f38690696c7e66be5c5474b54c6e791` |
| Licence | Apache-2.0 |
| Archive | `vidoseek_pdf_document.zip` |
| Annotations | `vidoseek.json` |

Every object has a declared byte size and SHA-256. The worker uses a fixed
HTTPS Hugging Face resolve URL, follows only Hugging Face HTTPS delivery
hosts, and rejects source drift, a size mismatch, a checksum mismatch, unsafe
ZIP paths, symlinks, duplicates, and archive bombs. No API request can supply
a source URL, storage coordinate, or model configuration.

## Recovery and privacy

Verified source files are SHA-addressed under the project-private output store.
On retry the worker checks the verified private copy first, so it does not
redownload an input that was already safely committed. Public Clone status
contains only its durable stage and a redacted failure code; it never reveals
private object paths, signed URLs, or credentials.

## Current runtime boundary

The Recipe Clone path is available independently of model credentials. A
visual Live Run is enabled only after the server has a visual parser and
multimodal retrieval runtime configured; it never falls back to the text RAG
runtime. The public Recipe and its hashes are published separately from the
upstream source bytes.

## Verify the public Recipe

After release, retrieve the Recipe through anonymous HTTPS:

```sh
base=https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs/visual-document-rag/1.0.0
curl -fsSLO "$base/SHA256SUMS"
shasum -a 256 -c SHA256SUMS
```

The repository provides matching guarded commands. Publishing is explicit and
requires object-storage credentials to be supplied by the release environment;
the command never reads credentials from the Recipe.

```sh
make visual-recipe-publish ORAG_PACK_PUBLISH=1 \
  VISUAL_RECIPE_ROOT=tutorial-recipes/visual-document-rag/1.0.0
make visual-recipe-verify \
  VISUAL_RECIPE_ROOT=tutorial-recipes/visual-document-rag/1.0.0
```

This verifies the Recipe declaration—not a mirror of ViDoSeek. A later
upstream revision or new source file requires a new tutorial version.
