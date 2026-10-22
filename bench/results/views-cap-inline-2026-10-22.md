# Views step 4: the 4 GiB cap check in line (2026-10-22)

Decision 0330. Step 3's cap check was a call (`cabi.len_cap`) made by every
runtime append and push entry, once per pushed byte in `Layout.format`'s
`put_name`; it showed as 6% of the profile (50 ms of 840 ms). The loop
invariant offset and mask the step 4 plan expected to hoist were not the
cost: the compiler's `hoist_invariants` already moves them, and `put_name`
computes the data address once per function. The test is now written in
each entry (`len_over` is only the fault).

Native arm64, instructions retired, `bench` program `timel` (300,000
iterations), the same std and the same formatter on both sides:

| mode | before step 3 | step 3 | cap in line |
| --- | --- | --- | --- |
| format_layout | 1,636 M | 1,785 M (+9.1%) | 1,708 M (+4.4%) |
| format | 5,612 M | 5,506 M (-1.9%) | 5,333 M (-5.0%) |
| parse | 3,397 M | 3,298 M (-2.9%) | 3,189 M (-6.1%) |
| parse_layout | 1,626 M | 1,651 M (+1.5%) | 1,588 M (-2.4%) |

format_layout keeps +4.4%: the masks of the packed length word in `reserve`
and `rs_push_byte` and the second mask in the entries, which the runtime
cannot drop while a view may be passed.
