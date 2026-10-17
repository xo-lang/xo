# snap OUT: write the selected thread's registers (DWARF numbers) and its
# stack, from sp to the end of sp's memory region (at most 512 KiB), as
# xo dev unwind's snapshot lines.
import lldb

def snap(debugger, command, result, internal_dict):
    out = command.strip()
    target = debugger.GetSelectedTarget()
    process = target.GetProcess()
    frame = process.GetSelectedThread().GetFrameAtIndex(0)
    lines = []
    gpr = frame.GetRegisters().GetValueAtIndex(0)
    names = {"fp": 29, "lr": 30, "sp": 31, "pc": -1}
    for i in range(29):
        names["x%d" % i] = i
    vals = {}
    for k in range(gpr.GetNumChildren()):
        r = gpr.GetChildAtIndex(k)
        if r.GetName() in names:
            vals[names[r.GetName()]] = r.GetValueAsUnsigned()
    for n in sorted(vals):
        lines.append("reg %d 0x%x" % (n, vals[n]))
    sp = vals[31]
    info = lldb.SBMemoryRegionInfo()
    process.GetMemoryRegionInfo(sp, info)
    end = min(info.GetRegionEnd(), sp + 512 * 1024)
    err = lldb.SBError()
    data = process.ReadMemory(sp, end - sp, err)
    if err.Success():
        lines.append("mem 0x%x %s" % (sp, data.hex()))
    with open(out, "w") as f:
        f.write("\n".join(lines) + "\n")
    result.AppendMessage("snap: %d registers, %d stack bytes" % (len(vals), len(data) if err.Success() else 0))

def __lldb_init_module(debugger, internal_dict):
    debugger.HandleCommand("command script add -f snap.snap snap")
