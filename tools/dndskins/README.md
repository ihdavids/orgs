# Sheet skin generators

The html character sheet's skins are the css files in `templates/dnd_skins/`;
those files are what ships and what the exporter reads. Most of their artwork
(bark frames, crystals, gears, ropes, the eldritch fissures...) is svg drawn by
the scripts here and inlined as data uris, so to change a drawing edit the
script or the `*.src.css` / `*.css.in` source and rebuild - editing the long
data uris in the shipped css by hand is not practical.

| Folder                | Skins                          | Rebuild                                                |
|-----------------------|--------------------------------|--------------------------------------------------------|
| `wildwood-strawberry` | wildwood, strawberry-meadow    | `python3 build.py wildwood ww_assets wildwood.src.css`<br>`python3 build.py strawberry-meadow sm_assets strawberry.src.css` |
| `gilded-pirate`       | gilded-tome, pirates-ledger    | `python3 build.py g` / `python3 build.py p`             |
| `deepdelve-tinker`    | deepdelve, tinkers-workshop    | `python3 gen_deepdelve.py` / `python3 gen_tinker.py`    |
| `eldritch`            | eldritch-pact                  | `sh build.sh` (`cracks.py` drew the fissures)           |

ink-marker, woodcut and field-journal are written by hand.

Each script writes into `templates/dnd_skins/`. After a rebuild run
`go test ./internal/common/dnd/ -run Skin` (every rule must stay scoped to its
skin) and look at the sheet: `/file/dndsheet?query=<sheet.org>&skin=<id>`
previews a skin without writing it to the character.
