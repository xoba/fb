data class Entry(val name: String, val bytes: Int)

fun main() {
    val files = listOf(Entry("notes.md", 120), Entry("letter.eml", 240))
    files.forEach { println("${it.name}: ${it.bytes} bytes") }
}
