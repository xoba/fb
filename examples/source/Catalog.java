public class Catalog {
    public static void main(String[] args) {
        String[] files = {"notes.md", "letter.eml"};
        for (String file : files) {
            System.out.println("Preview: " + file);
        }
    }
}
