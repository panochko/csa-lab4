; Стандартная библиотека. В машинный код попадают только вызванные функции.

(defun print-str (s)
  (let ((i 1)
        (n (load s)))
    (while (<= i n)
      (print-char (load (+ s (* i 4))))
      (setq i (+ i 1)))
    n))

; Читает строку до \n в pstr. Возвращает длину или -1, если не помещается.
(defun read-line (buf max)
  (let ((n 0)
        (c (read-char)))
    (while (and (/= c #\newline) (>= n 0))
      (if (< n max)
          (progn
            (setq n (+ n 1))
            (store (+ buf (* n 4)) c)
            (setq c (read-char)))
          (setq n -1)))
    (store buf (if (< n 0) 0 n))
    n))

(defun space? (c)
  (or (= c #\space) (= c #\newline)))

(defun digit? (c)
  (and (>= c #\0) (<= c #\9)))

; Читает десятичное число; разделитель после него поглощается.
(defun read-int ()
  (let ((c (read-char))
        (sign 1)
        (n 0))
    (while (space? c)
      (setq c (read-char)))
    (if (= c #\-)
        (progn
          (setq sign -1)
          (setq c (read-char))))
    (while (digit? c)
      (setq n (+ (* n 10) (- c #\0)))
      (setq c (read-char)))
    (* sign n)))

; Печать через отрицательные числа, чтобы работало и для -2147483648.
(defun print-neg (n)
  (if (<= n -10)
      (print-neg (/ n 10)))
  (print-char (- #\0 (mod n 10))))

(defun print-int (n)
  (if (< n 0)
      (progn
        (print-char #\-)
        (print-neg n))
      (print-neg (- 0 n)))
  n)

(defun print-hex (x)
  (let ((shift 28))
    (while (>= shift 0)
      (let ((d (logand (shr x shift) 15)))
        (print-char (if (< d 10)
                        (+ #\0 d)
                        (+ #\A (- d 10)))))
      (setq shift (- shift 4)))
    x))
