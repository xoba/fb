defmodule Catalog do
  @moduledoc "A small file-label example."

  def labels(names) do
    Enum.map(names, fn name -> "Preview: #{name}" end)
  end
end
