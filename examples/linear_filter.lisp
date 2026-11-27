; y[i] = 3*x[i] + 2*x[i-1] + x[i-2]. Соседние элементы читаются повторно,
; поэтому на этом алгоритме видна работа кэша.
(defun x-at (x i)
  (if (< i 0)
      0
      (load (+ x (* i 4)))))

(setq x (array 64))
(setq n (read-int))
(setq i 0)
(while (< i n)
  (store (+ x (* i 4)) (read-int))
  (setq i (+ i 1)))

(setq i 0)
(while (< i n)
  (print-int (+ (* 3 (x-at x i))
                (* 2 (x-at x (- i 1)))
                (x-at x (- i 2))))
  (print-char #\space)
  (setq i (+ i 1)))
