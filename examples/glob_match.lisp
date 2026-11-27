; Ввод: шаблон и текст, по строке. ? — один символ, * — любая подстрока.
(defun char-at (s i)
  (load (+ s (* (+ i 1) 4))))

; Рекурсивный поиск с возвратом: p — позиция в шаблоне, t — в тексте.
(defun match (pat text p t)
  (if (= p (load pat))
      (= t (load text))
      (if (= (char-at pat p) #\*)
          (or (match pat text (+ p 1) t)
              (and (< t (load text))
                   (match pat text p (+ t 1))))
          (and (< t (load text))
               (or (= (char-at pat p) #\?)
                   (= (char-at pat p) (char-at text t)))
               (match pat text (+ p 1) (+ t 1))))))

(setq pattern (array 32))
(setq text (array 32))
(if (or (< (read-line pattern 31) 0)
        (< (read-line text 31) 0))
    (print-int -858993460)
    (print-int (if (match pattern text 0 0) 1 0)))
