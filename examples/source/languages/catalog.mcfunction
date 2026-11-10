# Announce the entries of the example file catalog.
scoreboard objectives add catalog_bytes dummy
scoreboard players set notes catalog_bytes 120
scoreboard players set letter catalog_bytes 240
tellraw @a {"text":"notes.md: 120 bytes","color":"gold"}
tellraw @a [{"text":"letter.eml: "},{"score":{"name":"letter","objective":"catalog_bytes"}},{"text":" bytes"}]
execute as @a[tag=librarian] at @s run playsound minecraft:block.note_block.chime master @s ~ ~ ~
