module Main where

files :: [String]
files = ["notes.md", "letter.eml"]

main :: IO ()
main = mapM_ (putStrLn . ("Preview: " ++)) files
