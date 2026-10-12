(ns catalog)

(def files [{:name "notes.md" :kind :document}
            {:name "letter.eml" :kind :message}])

(doseq [{:keys [name kind]} files]
  (println name "is a" (clojure.core/name kind)))
