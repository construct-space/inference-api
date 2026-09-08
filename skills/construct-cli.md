# Construct CLI — canonical commands

Verified against `construct --version 1.7.5`. Use `bun` everywhere; never `npm`.

## Install + setup

```bash
bun add -g @construct-space/cli      # global install
construct --version                  # verify
construct login                      # opens portal; or --token <token> for headless
construct whoami                     # signed-in user + active org
```

## Scaffold a new space

```bash
construct scaffold my-space          # minimal
construct scaffold my-space --full   # multiple pages + skills + widget templates
construct scaffold my-space --with-tests
cd my-space
bun install
```

## Develop

```bash
construct dev                        # file watch + live reload
construct build                      # generate src/entry.ts + run vite
construct build --entry-only         # only generate entry.ts
construct install                    # install built space into Construct
construct validate                   # validate space.manifest.json
construct check                      # vue-tsc + eslint
construct clean                      # remove build artifacts
construct clean --all                # also remove node_modules + lockfile
```

## Graph

```bash
construct graph init                                       # initialize in current space
construct graph generate <Model> field:type[:modifier]     # scaffold a model
construct graph g <Model> field:type ...                   # short alias
construct graph push                                       # register with Graph service
construct graph migrate                                    # diff schema
construct graph migrate --apply                            # apply destructive changes
construct graph fork <new-space-id>                        # change graph id
construct graph spaces                                     # list org's spaces
construct graph spaces --json
construct graph install <space-id>                         # install a published space for the org
construct graph uninstall <space-id>                       # uninstall (data preserved)
construct graph distribution <space-id> public|org_allowlist|private
construct graph allowlist add <space-id> <org-id>
construct graph allowlist rm <space-id> <org-id>
construct graph bundles                                    # publisher bundle management
```

`construct graph g` field syntax: `name:type[:modifier1:modifier2]`

Examples:

```bash
construct graph g User name:string email:string:required:unique
construct graph g Post title:string body:string published:boolean
construct graph g Comment body:string post:belongsTo:Post
construct graph g Task status:enum:todo,doing,done priority:int
```

Add access rules with `--access`:

```bash
construct graph g Note title:string \
  --access read:member,create:member,update:owner,delete:admin
```

## Publish

```bash
construct publish                            # publish to registry (preserves visibility)
construct publish -y --bump patch            # auto-bump version
construct publish --bump minor
construct publish --bump major
construct publish --private                  # org-private (requires org publisher key)
construct publish --public                   # flip private back to public
```

## End-to-end from scratch

```bash
construct scaffold notes
cd notes
bun install
construct graph init
construct graph g Note title:string:required content:string pinned:boolean
construct graph push
# write src/actions.ts + src/pages/index.vue
construct build
construct install
construct dev    # iterate
construct publish -y --bump patch
```

## Bun cheatsheet

```bash
bun install                          # install deps
bun add @construct-space/ui          # add dep
bun add -d eslint                    # add dev dep
bun remove some-pkg
bun run build                        # run package.json script
bun pm ls                            # tree
bun x <pkg>                          # one-shot run
```

Never `npm install` in a Construct space — the workspace expects `bun.lock`.
