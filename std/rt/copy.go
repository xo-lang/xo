package rt

// Copying between streams and files (decision 0593): Go's io.Copy, 32 KiB
// reads, each piece written in full, the failing side named by CopyErr.

const copyBuf = 32768

func copyRead[R, W any](e R) CopyErr[R, W] {
	return CopyErr[R, W]{Tag: 1, P: CopyErr_Read[R, W]{F_err: e}}
}

func copyWrite[R, W any](e W) CopyErr[R, W] {
	return CopyErr[R, W]{Tag: 2, P: CopyErr_Write[R, W]{F_err: e}}
}

// copyLoop reads with read until it ends and writes each piece with write.
func copyLoop[R, W any](read func() Result[Option[Bytes], R], write func(Bytes) Result[Unit, W]) Result[int64, CopyErr[R, W]] {
	var n int64
	for {
		r := read()
		if !r.Ok {
			return ErrTr[int64](copyRead[R, W](r.E), r.Tr)
		}
		if !r.V.Some {
			return Ok[int64, CopyErr[R, W]](n)
		}
		w := write(r.V.V)
		if !w.Ok {
			return ErrTr[int64](copyWrite[R](w.E), w.Tr)
		}
		n += int64(len(r.V.V))
	}
}

// Prelude_copy_stream is `copy_stream(dst, src)`.
func Prelude_copy_stream(tk *Task, dst, src Stream) Result[int64, CopyErr[NetErr, NetErr]] {
	return copyLoop(
		func() Result[Option[Bytes], NetErr] { return src.M_read(tk, copyBuf) },
		func(b Bytes) Result[Unit, NetErr] { return dst.M_write(tk, b) })
}

// M_copy_to is `f.copy_to(dst)`.
func (f File) M_copy_to(tk *Task, dst Stream) Result[int64, CopyErr[FsErr, NetErr]] {
	return copyLoop(
		func() Result[Option[Bytes], FsErr] { return f.M_read(tk, copyBuf) },
		func(b Bytes) Result[Unit, NetErr] { return dst.M_write(tk, b) })
}

// M_copy_from is `f.copy_from(src)`.
func (f File) M_copy_from(tk *Task, src Stream) Result[int64, CopyErr[NetErr, FsErr]] {
	return copyLoop(
		func() Result[Option[Bytes], NetErr] { return src.M_read(tk, copyBuf) },
		func(b Bytes) Result[Unit, FsErr] { return f.M_write(tk, b) })
}
