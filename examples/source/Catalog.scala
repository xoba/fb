object Catalog {
  case class Entry(name: String, bytes: Int)

  def main(args: Array[String]): Unit = {
    val files = List(Entry("notes.md", 120), Entry("letter.eml", 240))
    files.foreach(file => println(s"${file.name}: ${file.bytes} bytes"))
  }
}
