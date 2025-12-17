        ; Quick sort demo for m68kemu
        ; Sorts the longword array defined at the end of the file in place

        ORG     $2000
start:  LEA     array,A0           ; A0 points to the start of the array
        MOVEQ   #0,D6              ; low index
        MOVE.L  #(array_end-array)/4-1,D7
        BSR     qsort
halt:   BRA     halt               ; loop forever once sorting is done

; qsort(A0 base pointer, D6 low index, D7 high index)
qsort:
        CMP.L   D7,D6
        BGE.S   qreturn            ; nothing to sort when low >= high
        MOVEM.L D3-D7/A0-A1,-(A7)

        MOVE.L  D6,D3              ; i = low
        MOVE.L  D7,D4              ; j = high

        MOVE.L  D6,D5
        ADD.L   D7,D5
        LSR.L   #1,D5              ; pivot index = (low + high) / 2
        MOVE.L  D5,D0
        LSL.L   #2,D0
        MOVE.L  (A0,D0.L),D5       ; pivot value

partition:
left_scan:
        MOVE.L  D3,D0
        LSL.L   #2,D0
        MOVE.L  (A0,D0.L),D2
        CMP.L   D5,D2
        BGE.S   left_done          ; stop when a[i] >= pivot
        ADDQ.L  #1,D3
        BRA     left_scan
left_done:
right_scan:
        MOVE.L  D4,D1
        LSL.L   #2,D1
        MOVE.L  (A0,D1.L),A1
        CMP.L   A1,D5
        BGE.S   right_done         ; stop when a[j] <= pivot
        SUBQ.L  #1,D4
        BRA     right_scan
right_done:
        CMP.L   D4,D3
        BGT.S   partition_exit     ; pointers crossed

        MOVE.L  D3,D0
        LSL.L   #2,D0
        MOVE.L  D4,D1
        LSL.L   #2,D1
        MOVE.L  (A0,D0.L),D2       ; left value
        MOVE.L  (A0,D1.L),A1       ; right value

        MOVE.L  A1,(A0,D0.L)
        MOVE.L  D2,(A0,D1.L)

        ADDQ.L  #1,D3
        SUBQ.L  #1,D4
        BRA     left_scan

partition_exit:
        MOVE.L  D7,-(A7)           ; save original high for the right branch
        CMP.L   D4,D6
        BLE.S   skip_left
        MOVE.L  D4,D7              ; high = j
        BSR     qsort              ; sort the left partition
skip_left:
        MOVE.L  (A7)+,D7
        CMP.L   D7,D3
        BLE.S   skip_right
        MOVE.L  D3,D6              ; low = i
        BSR     qsort              ; sort the right partition
skip_right:
        MOVEM.L (A7)+,D3-D7/A0-A1
qreturn:
        RTS

        EVEN
array:  DC.L    9,1,7,3,8,2,6,5,4,0
array_end:
