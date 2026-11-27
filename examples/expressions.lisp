; Всё является выражением; рекурсия; addn/muln с переменным числом операндов.
(defun fib (n)
  (if (< n 2)
      n
      (+ (fib (- n 1)) (fib (- n 2)))))

(defun show (x)
  (print-int x)
  (print-char #\newline))

(setq x 1)
(print-str (if (= 1 x) "T\n" "F\n"))
(setq x (if (= 1 x) 2 3))
(show x)
(show (setq x 13))
(setq i 0)
(setq x (while (< i 5)
          (setq i (+ i 1))
          (* i i)))
(show x)
(show (+ x i 100 1000))
(show (* 2 3 4 5))
(show (+ (fib 3) (fib 4) (fib 5)))
(show (fib 12))
