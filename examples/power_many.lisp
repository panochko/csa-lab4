; Ввод: count, затем count пар (основание, показатель).
(defun overflow ()
  -858993460) ; 0xCCCCCCCC

; Степень или код переполнения.
(defun power (base exp)
  (let ((result 1)
        (i 0)
        (bad 0))
    (while (and (< i exp) (not bad))
      (let ((next (* result base)))
        (if (and (/= base 0) (/= (/ next base) result))
            (setq bad 1)
            (setq result next)))
      (setq i (+ i 1)))
    (if bad (overflow) result)))

(defun solve (count results)
  (let ((i 0)
        (ok 1))
    (while (< i count)
      (let ((base (read-int))
            (exp (read-int)))
        (if (< exp 0)
            (setq ok 0)
            (store (+ results (* i 4)) (power base exp))))
      (setq i (+ i 1)))
    ok))

(defun print-line (x)
  (print-int x)
  (print-char #\newline))

(setq results (array 32))
(setq count (read-int))
(if (and (> count 0) (solve count results))
    (progn
      (setq i 0)
      (while (< i count)
        (print-line (load (+ results (* i 4))))
        (setq i (+ i 1))))
    (print-line -1))
