; 64-битное число хранится двумя словами: [младшее, старшее].
(defun set64 (x lo hi)
  (store x lo)
  (store (+ x 4) hi))

(defun print64 (x)
  (print-str "0x")
  (print-hex (load (+ x 4)))
  (print-hex (load x))
  (print-char #\newline))

; x = x * k, где k — 32-битное беззнаковое
(defun mul64 (x k)
  (let ((lo (load x))
        (hi (load (+ x 4))))
    (set64 x
           (* lo k)
           (+ (* hi k) (mulh lo k)))))

(setq a (array 2))
(setq b (array 2))

; перенос из младшего слова: add + adc
(set64 a 0xFFFFFFFF 1)
(set64 b 1 2)
(print64 (add64 a b))

; заём: sub + sbc
(set64 a 0 5)
(set64 b 1 0)
(print64 (sub64 a b))

; 20! не помещается в 32 бита
(set64 a 1 0)
(setq i 2)
(while (<= i 20)
  (mul64 a i)
  (setq i (+ i 1)))
(print64 a)
