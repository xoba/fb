Feature: Browse example files
  Browsing a folder makes its readable files easy to discover.

  Scenario: Open a Markdown document
    Given a folder containing "notes.md"
    When I select "notes.md"
    Then I see the rendered document
    And the original file is unchanged
