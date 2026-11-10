// Print labels for an in-memory file catalog.
class val Entry
  let name: String
  let bytes: USize

  new val create(name': String, bytes': USize) =>
    name = name'
    bytes = bytes'

  fun label(): String =>
    name + ": " + bytes.string() + " bytes"

actor Main
  new create(env: Env) =>
    let files = [Entry("notes.md", 120); Entry("letter.eml", 240)]
    var total: USize = 0
    for entry in files.values() do
      env.out.print(entry.label())
      total = total + entry.bytes
    end
    env.out.print("total: " + total.string() + " bytes")
