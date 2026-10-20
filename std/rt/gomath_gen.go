// Code generated from Go 1.26's math package by the rewriter in
// scripts/gomath (pure Go, assembly removed); DO NOT EDIT. Decision 0287.

package rt

import (
	"math"
	"math/bits"
)

var _ = bits.Len64

func gmAbs(x float64) float64 {
	return gmFloat64frombits(gmFloat64bits(x) &^ (1 << 63))
}

func gmAcosh(x float64) float64 {
	if gmhaveArchAcosh {
		return gmarchAcosh(x)
	}
	return gmacosh(x)
}

func gmacosh(x float64) float64 {
	const Large = 1 << 28

	switch {
	case x < 1 || gmIsNaN(x):
		return gmNaN()
	case x == 1:
		return 0
	case x >= Large:
		return gmLog(x) + gmLn2
	case x > 2:
		return gmLog(float64(2*x) - 1/(x+gmSqrt(float64(x*x)-1)))
	}
	t := x - 1
	return gmLog1p(t + gmSqrt(float64(2*t)+float64(t*t)))
}

func gmAsin(x float64) float64 {
	if gmhaveArchAsin {
		return gmarchAsin(x)
	}
	return gmasin(x)
}

func gmasin(x float64) float64 {
	if x == 0 {
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if x > 1 {
		return gmNaN()
	}

	temp := gmSqrt(1 - float64(x*x))
	if x > 0.7 {
		temp = gmPi/2 - gmsatan(temp/x)
	} else {
		temp = gmsatan(x / temp)
	}

	if sign {
		temp = -temp
	}
	return temp
}

func gmAcos(x float64) float64 {
	if gmhaveArchAcos {
		return gmarchAcos(x)
	}
	return gmacos(x)
}

func gmacos(x float64) float64 {
	return gmPi/2 - gmAsin(x)
}

func gmAsinh(x float64) float64 {
	if gmhaveArchAsinh {
		return gmarchAsinh(x)
	}
	return gmasinh(x)
}

func gmasinh(x float64) float64 {
	const (
		Ln2      = 6.93147180559945286227e-01
		NearZero = 1.0 / (1 << 28)
		Large    = 1 << 28
	)

	if gmIsNaN(x) || gmIsInf(x, 0) {
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var temp float64
	switch {
	case x > Large:
		temp = gmLog(x) + Ln2
	case x > 2:
		temp = gmLog(float64(2*x) + 1/(gmSqrt(float64(x*x)+1)+x))
	case x < NearZero:
		temp = x
	default:
		temp = gmLog1p(x + float64(x*x)/(1+gmSqrt(1+float64(x*x))))
	}
	if sign {
		temp = -temp
	}
	return temp
}

func gmxatan(x float64) float64 {
	const (
		P0 = -8.750608600031904122785e-01
		P1 = -1.615753718733365076637e+01
		P2 = -7.500855792314704667340e+01
		P3 = -1.228866684490136173410e+02
		P4 = -6.485021904942025371773e+01
		Q0 = +2.485846490142306297962e+01
		Q1 = +1.650270098316988542046e+02
		Q2 = +4.328810604912902668951e+02
		Q3 = +4.853903996359136964868e+02
		Q4 = +1.945506571482613964425e+02
	)
	z := float64(x * x)
	z = float64(z*(float64((float64((float64((float64(P0*z)+P1)*z)+P2)*z)+P3)*z)+P4)) / (float64((float64((float64((float64((z+Q0)*z)+Q1)*z)+Q2)*z)+Q3)*z) + Q4)
	z = float64(x*z) + x
	return z
}

func gmsatan(x float64) float64 {
	const (
		Morebits = 6.123233995736765886130e-17
		Tan3pio8 = 2.41421356237309504880
	)
	if x <= 0.66 {
		return gmxatan(x)
	}
	if x > Tan3pio8 {
		return gmPi/2 - gmxatan(1/x) + Morebits
	}
	return gmPi/4 + gmxatan((x-1)/(x+1)) + 0.5*Morebits
}

func gmAtan(x float64) float64 {
	if gmhaveArchAtan {
		return gmarchAtan(x)
	}
	return gmatan(x)
}

func gmatan(x float64) float64 {
	if x == 0 {
		return x
	}
	if x > 0 {
		return gmsatan(x)
	}
	return -gmsatan(-x)
}

func gmAtan2(y, x float64) float64 {
	if gmhaveArchAtan2 {
		return gmarchAtan2(y, x)
	}
	return gmatan2(y, x)
}

func gmatan2(y, x float64) float64 {

	switch {
	case gmIsNaN(y) || gmIsNaN(x):
		return gmNaN()
	case y == 0:
		if x >= 0 && !gmSignbit(x) {
			return gmCopysign(0, y)
		}
		return gmCopysign(gmPi, y)
	case x == 0:
		return gmCopysign(gmPi/2, y)
	case gmIsInf(x, 0):
		if gmIsInf(x, 1) {
			switch {
			case gmIsInf(y, 0):
				return gmCopysign(gmPi/4, y)
			default:
				return gmCopysign(0, y)
			}
		}
		switch {
		case gmIsInf(y, 0):
			return gmCopysign(3*gmPi/4, y)
		default:
			return gmCopysign(gmPi, y)
		}
	case gmIsInf(y, 0):
		return gmCopysign(gmPi/2, y)
	}

	q := gmAtan(y / x)
	if x < 0 {
		if q <= 0 {
			return q + gmPi
		}
		return q - gmPi
	}
	return q
}

func gmAtanh(x float64) float64 {
	if gmhaveArchAtanh {
		return gmarchAtanh(x)
	}
	return gmatanh(x)
}

func gmatanh(x float64) float64 {
	const NearZero = 1.0 / (1 << 28)

	switch {
	case x < -1 || x > 1 || gmIsNaN(x):
		return gmNaN()
	case x == 1:
		return gmInf(1)
	case x == -1:
		return gmInf(-1)
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var temp float64
	switch {
	case x < NearZero:
		temp = x
	case x < 0.5:
		temp = x + x
		temp = float64(0.5 * gmLog1p(temp+float64(temp*x)/(1-x)))
	default:
		temp = float64(0.5 * gmLog1p((x+x)/(1-x)))
	}
	if sign {
		temp = -temp
	}
	return temp
}

const (
	gmuvnan    = 0x7FF8000000000001
	gmuvinf    = 0x7FF0000000000000
	gmuvneginf = 0xFFF0000000000000
	gmuvone    = 0x3FF0000000000000
	gmmask     = 0x7FF
	gmshift    = 64 - 11 - 1
	gmbias     = 1023
	gmsignMask = 1 << 63
	gmfracMask = 1<<gmshift - 1
)

func gmInf(sign int) float64 {
	var v uint64
	if sign >= 0 {
		v = gmuvinf
	} else {
		v = gmuvneginf
	}
	return gmFloat64frombits(v)
}

func gmNaN() float64 { return gmFloat64frombits(gmuvnan) }

func gmIsNaN(f float64) (is bool) {

	return f != f
}

func gmIsInf(f float64, sign int) bool {

	if sign == 0 {
		f = gmAbs(f)
	} else if sign < 0 {
		f = -f
	}
	return f > gmMaxFloat64
}

func gmnormalize(x float64) (y float64, exp int) {
	const SmallestNormal = 2.2250738585072014e-308
	if gmAbs(x) < SmallestNormal {
		return float64(x * (1 << 52)), -52
	}
	return x, 0
}

func gmCbrt(x float64) float64 {
	if gmhaveArchCbrt {
		return gmarchCbrt(x)
	}
	return gmcbrt(x)
}

func gmcbrt(x float64) float64 {
	const (
		B1             = 715094163
		B2             = 696219795
		C              = 5.42857142857142815906e-01
		D              = -7.05306122448979611050e-01
		E              = 1.41428571428571436819e+00
		F              = 1.60714285714285720630e+00
		G              = 3.57142857142857150787e-01
		SmallestNormal = 2.22507385850720138309e-308
	)

	switch {
	case x == 0 || gmIsNaN(x) || gmIsInf(x, 0):
		return x
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}

	t := gmFloat64frombits(gmFloat64bits(x)/3 + B1<<32)
	if x < SmallestNormal {

		t = float64(1 << 54)
		t *= x
		t = gmFloat64frombits(gmFloat64bits(t)/3 + B2<<32)
	}

	r := float64(t*t) / x
	s := C + float64(r*t)
	t *= G + F/(s+E+D/s)

	t = gmFloat64frombits(gmFloat64bits(t)&(0xFFFFFFFFC<<28) + 1<<30)

	s = float64(t * t)
	r = x / s
	w := t + t
	r = (r - t) / (w + r)
	t = t + float64(t*r)

	if sign {
		t = -t
	}
	return t
}

const (
	gmE   = 2.71828182845904523536028747135266249775724709369995957496696763
	gmPi  = 3.14159265358979323846264338327950288419716939937510582097494459
	gmPhi = 1.61803398874989484820458683436563811772030917980576286213544862

	gmSqrt2   = 1.41421356237309504880168872420969807856967187537694807317667974
	gmSqrtE   = 1.64872127070012814684865078781416357165377610071014801157507931
	gmSqrtPi  = 1.77245385090551602729816748334114518279754945612238712821380779
	gmSqrtPhi = 1.27201964951406896425242246173749149171560804184009624861664038

	gmLn2    = 0.693147180559945309417232121458176568075500134360255254120680009
	gmLog2E  = 1 / gmLn2
	gmLn10   = 2.30258509299404568401799145468436420760110148862877297603332790
	gmLog10E = 1 / gmLn10
)

const (
	gmMaxFloat32             = 0x1p127 * (1 + (1 - 0x1p-23))
	gmSmallestNonzeroFloat32 = 0x1p-126 * 0x1p-23

	gmMaxFloat64             = 0x1p1023 * (1 + (1 - 0x1p-52))
	gmSmallestNonzeroFloat64 = 0x1p-1022 * 0x1p-52
)

const (
	gmintSize = 32 << (^uint(0) >> 63)

	gmMaxInt    = 1<<(gmintSize-1) - 1
	gmMinInt    = -1 << (gmintSize - 1)
	gmMaxInt8   = 1<<7 - 1
	gmMinInt8   = -1 << 7
	gmMaxInt16  = 1<<15 - 1
	gmMinInt16  = -1 << 15
	gmMaxInt32  = 1<<31 - 1
	gmMinInt32  = -1 << 31
	gmMaxInt64  = 1<<63 - 1
	gmMinInt64  = -1 << 63
	gmMaxUint   = 1<<gmintSize - 1
	gmMaxUint8  = 1<<8 - 1
	gmMaxUint16 = 1<<16 - 1
	gmMaxUint32 = 1<<32 - 1
	gmMaxUint64 = 1<<64 - 1
)

func gmCopysign(f, sign float64) float64 {
	const signBit = 1 << 63
	return gmFloat64frombits(gmFloat64bits(f)&^signBit | gmFloat64bits(sign)&signBit)
}

func gmDim(x, y float64) float64 {

	v := x - y
	if v <= 0 {

		return 0
	}

	return v
}

func gmMax(x, y float64) float64 {
	if gmhaveArchMax {
		return gmarchMax(x, y)
	}
	return gmmax(x, y)
}

func gmmax(x, y float64) float64 {

	switch {
	case gmIsInf(x, 1) || gmIsInf(y, 1):
		return gmInf(1)
	case gmIsNaN(x) || gmIsNaN(y):
		return gmNaN()
	case x == 0 && x == y:
		if gmSignbit(x) {
			return y
		}
		return x
	}
	if x > y {
		return x
	}
	return y
}

func gmMin(x, y float64) float64 {
	if gmhaveArchMin {
		return gmarchMin(x, y)
	}
	return gmmin(x, y)
}

func gmmin(x, y float64) float64 {

	switch {
	case gmIsInf(x, -1) || gmIsInf(y, -1):
		return gmInf(-1)
	case gmIsNaN(x) || gmIsNaN(y):
		return gmNaN()
	case x == 0 && x == y:
		if gmSignbit(x) {
			return x
		}
		return y
	}
	if x < y {
		return x
	}
	return y
}

const gmhaveArchMax = false

func gmarchMax(x, y float64) float64 {
	panic("not implemented")
}

const gmhaveArchMin = false

func gmarchMin(x, y float64) float64 {
	panic("not implemented")
}

const (
	gmerx = 8.45062911510467529297e-01

	gmefx  = 1.28379167095512586316e-01
	gmefx8 = 1.02703333676410069053e+00
	gmpp0  = 1.28379167095512558561e-01
	gmpp1  = -3.25042107247001499370e-01
	gmpp2  = -2.84817495755985104766e-02
	gmpp3  = -5.77027029648944159157e-03
	gmpp4  = -2.37630166566501626084e-05
	gmqq1  = 3.97917223959155352819e-01
	gmqq2  = 6.50222499887672944485e-02
	gmqq3  = 5.08130628187576562776e-03
	gmqq4  = 1.32494738004321644526e-04
	gmqq5  = -3.96022827877536812320e-06

	gmpa0 = -2.36211856075265944077e-03
	gmpa1 = 4.14856118683748331666e-01
	gmpa2 = -3.72207876035701323847e-01
	gmpa3 = 3.18346619901161753674e-01
	gmpa4 = -1.10894694282396677476e-01
	gmpa5 = 3.54783043256182359371e-02
	gmpa6 = -2.16637559486879084300e-03
	gmqa1 = 1.06420880400844228286e-01
	gmqa2 = 5.40397917702171048937e-01
	gmqa3 = 7.18286544141962662868e-02
	gmqa4 = 1.26171219808761642112e-01
	gmqa5 = 1.36370839120290507362e-02
	gmqa6 = 1.19844998467991074170e-02

	gmra0 = -9.86494403484714822705e-03
	gmra1 = -6.93858572707181764372e-01
	gmra2 = -1.05586262253232909814e+01
	gmra3 = -6.23753324503260060396e+01
	gmra4 = -1.62396669462573470355e+02
	gmra5 = -1.84605092906711035994e+02
	gmra6 = -8.12874355063065934246e+01
	gmra7 = -9.81432934416914548592e+00
	gmsa1 = 1.96512716674392571292e+01
	gmsa2 = 1.37657754143519042600e+02
	gmsa3 = 4.34565877475229228821e+02
	gmsa4 = 6.45387271733267880336e+02
	gmsa5 = 4.29008140027567833386e+02
	gmsa6 = 1.08635005541779435134e+02
	gmsa7 = 6.57024977031928170135e+00
	gmsa8 = -6.04244152148580987438e-02

	gmrb0 = -9.86494292470009928597e-03
	gmrb1 = -7.99283237680523006574e-01
	gmrb2 = -1.77579549177547519889e+01
	gmrb3 = -1.60636384855821916062e+02
	gmrb4 = -6.37566443368389627722e+02
	gmrb5 = -1.02509513161107724954e+03
	gmrb6 = -4.83519191608651397019e+02
	gmsb1 = 3.03380607434824582924e+01
	gmsb2 = 3.25792512996573918826e+02
	gmsb3 = 1.53672958608443695994e+03
	gmsb4 = 3.19985821950859553908e+03
	gmsb5 = 2.55305040643316442583e+03
	gmsb6 = 4.74528541206955367215e+02
	gmsb7 = -2.24409524465858183362e+01
)

func gmErf(x float64) float64 {
	if gmhaveArchErf {
		return gmarchErf(x)
	}
	return gmerf(x)
}

func gmerf(x float64) float64 {
	const (
		VeryTiny = 2.848094538889218e-306
		Small    = 1.0 / (1 << 28)
	)

	switch {
	case gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return 1
	case gmIsInf(x, -1):
		return -1
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if x < 0.84375 {
		var temp float64
		if x < Small {
			if x < VeryTiny {
				temp = float64(0.125 * (float64(8.0*x) + float64(gmefx8*x)))
			} else {
				temp = x + float64(gmefx*x)
			}
		} else {
			z := float64(x * x)
			r := gmpp0 + float64(z*(gmpp1+float64(z*(gmpp2+float64(z*(gmpp3+float64(z*gmpp4)))))))
			s := 1 + float64(z*(gmqq1+float64(z*(gmqq2+float64(z*(gmqq3+float64(z*(gmqq4+float64(z*gmqq5)))))))))
			y := r / s
			temp = x + float64(x*y)
		}
		if sign {
			return -temp
		}
		return temp
	}
	if x < 1.25 {
		s := x - 1
		P := gmpa0 + float64(s*(gmpa1+float64(s*(gmpa2+float64(s*(gmpa3+float64(s*(gmpa4+float64(s*(gmpa5+float64(s*gmpa6)))))))))))
		Q := 1 + float64(s*(gmqa1+float64(s*(gmqa2+float64(s*(gmqa3+float64(s*(gmqa4+float64(s*(gmqa5+float64(s*gmqa6)))))))))))
		if sign {
			return -gmerx - P/Q
		}
		return gmerx + P/Q
	}
	if x >= 6 {
		if sign {
			return -1
		}
		return 1
	}
	s := 1 / (float64(x * x))
	var R, S float64
	if x < 1/0.35 {
		R = gmra0 + float64(s*(gmra1+float64(s*(gmra2+float64(s*(gmra3+float64(s*(gmra4+float64(s*(gmra5+float64(s*(gmra6+float64(s*gmra7)))))))))))))
		S = 1 + float64(s*(gmsa1+float64(s*(gmsa2+float64(s*(gmsa3+float64(s*(gmsa4+float64(s*(gmsa5+float64(s*(gmsa6+float64(s*(gmsa7+float64(s*gmsa8)))))))))))))))
	} else {
		R = gmrb0 + float64(s*(gmrb1+float64(s*(gmrb2+float64(s*(gmrb3+float64(s*(gmrb4+float64(s*(gmrb5+float64(s*gmrb6)))))))))))
		S = 1 + float64(s*(gmsb1+float64(s*(gmsb2+float64(s*(gmsb3+float64(s*(gmsb4+float64(s*(gmsb5+float64(s*(gmsb6+float64(s*gmsb7)))))))))))))
	}
	z := gmFloat64frombits(gmFloat64bits(x) & 0xffffffff00000000)
	r := float64(gmExp(float64(-z*z)-0.5625) * gmExp(float64((z-x)*(z+x))+R/S))
	if sign {
		return r/x - 1
	}
	return 1 - r/x
}

func gmErfc(x float64) float64 {
	if gmhaveArchErfc {
		return gmarchErfc(x)
	}
	return gmerfc(x)
}

func gmerfc(x float64) float64 {
	const Tiny = 1.0 / (1 << 56)

	switch {
	case gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return 0
	case gmIsInf(x, -1):
		return 2
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if x < 0.84375 {
		var temp float64
		if x < Tiny {
			temp = x
		} else {
			z := float64(x * x)
			r := gmpp0 + float64(z*(gmpp1+float64(z*(gmpp2+float64(z*(gmpp3+float64(z*gmpp4)))))))
			s := 1 + float64(z*(gmqq1+float64(z*(gmqq2+float64(z*(gmqq3+float64(z*(gmqq4+float64(z*gmqq5)))))))))
			y := r / s
			if x < 0.25 {
				temp = x + float64(x*y)
			} else {
				temp = 0.5 + (float64(x*y) + (x - 0.5))
			}
		}
		if sign {
			return 1 + temp
		}
		return 1 - temp
	}
	if x < 1.25 {
		s := x - 1
		P := gmpa0 + float64(s*(gmpa1+float64(s*(gmpa2+float64(s*(gmpa3+float64(s*(gmpa4+float64(s*(gmpa5+float64(s*gmpa6)))))))))))
		Q := 1 + float64(s*(gmqa1+float64(s*(gmqa2+float64(s*(gmqa3+float64(s*(gmqa4+float64(s*(gmqa5+float64(s*gmqa6)))))))))))
		if sign {
			return 1 + gmerx + P/Q
		}
		return 1 - gmerx - P/Q

	}
	if x < 28 {
		s := 1 / (float64(x * x))
		var R, S float64
		if x < 1/0.35 {
			R = gmra0 + float64(s*(gmra1+float64(s*(gmra2+float64(s*(gmra3+float64(s*(gmra4+float64(s*(gmra5+float64(s*(gmra6+float64(s*gmra7)))))))))))))
			S = 1 + float64(s*(gmsa1+float64(s*(gmsa2+float64(s*(gmsa3+float64(s*(gmsa4+float64(s*(gmsa5+float64(s*(gmsa6+float64(s*(gmsa7+float64(s*gmsa8)))))))))))))))
		} else {
			if sign && x > 6 {
				return 2
			}
			R = gmrb0 + float64(s*(gmrb1+float64(s*(gmrb2+float64(s*(gmrb3+float64(s*(gmrb4+float64(s*(gmrb5+float64(s*gmrb6)))))))))))
			S = 1 + float64(s*(gmsb1+float64(s*(gmsb2+float64(s*(gmsb3+float64(s*(gmsb4+float64(s*(gmsb5+float64(s*(gmsb6+float64(s*gmsb7)))))))))))))
		}
		z := gmFloat64frombits(gmFloat64bits(x) & 0xffffffff00000000)
		r := float64(gmExp(float64(-z*z)-0.5625) * gmExp(float64((z-x)*(z+x))+R/S))
		if sign {
			return 2 - r/x
		}
		return r / x
	}
	if sign {
		return 2
	}
	return 0
}

const (
	gma0 = 1.1975323115670912564578e0
	gma1 = 4.7072688112383978012285e1
	gma2 = 6.9706266534389598238465e2
	gma3 = 4.8548868893843886794648e3
	gma4 = 1.6235862515167575384252e4
	gma5 = 2.3782041382114385731252e4
	gma6 = 1.1819493347062294404278e4
	gma7 = 8.8709406962545514830200e2
	gmb0 = 1.0000000000000000000e0
	gmb1 = 4.2313330701600911252e1
	gmb2 = 6.8718700749205790830e2
	gmb3 = 5.3941960214247511077e3
	gmb4 = 2.1213794301586595867e4
	gmb5 = 3.9307895800092710610e4
	gmb6 = 2.8729085735721942674e4
	gmb7 = 5.2264952788528545610e3

	gmc0 = 1.42343711074968357734e0
	gmc1 = 4.63033784615654529590e0
	gmc2 = 5.76949722146069140550e0
	gmc3 = 3.64784832476320460504e0
	gmc4 = 1.27045825245236838258e0
	gmc5 = 2.41780725177450611770e-1
	gmc6 = 2.27238449892691845833e-2
	gmc7 = 7.74545014278341407640e-4
	gmd0 = 1.4142135623730950488016887e0
	gmd1 = 2.9036514445419946173133295e0
	gmd2 = 2.3707661626024532365971225e0
	gmd3 = 9.7547832001787427186894837e-1
	gmd4 = 2.0945065210512749128288442e-1
	gmd5 = 2.1494160384252876777097297e-2
	gmd6 = 7.7441459065157709165577218e-4
	gmd7 = 1.4859850019840355905497876e-9

	gme0 = 6.65790464350110377720e0
	gme1 = 5.46378491116411436990e0
	gme2 = 1.78482653991729133580e0
	gme3 = 2.96560571828504891230e-1
	gme4 = 2.65321895265761230930e-2
	gme5 = 1.24266094738807843860e-3
	gme6 = 2.71155556874348757815e-5
	gme7 = 2.01033439929228813265e-7
	gmf0 = 1.414213562373095048801689e0
	gmf1 = 8.482908416595164588112026e-1
	gmf2 = 1.936480946950659106176712e-1
	gmf3 = 2.103693768272068968719679e-2
	gmf4 = 1.112800997078859844711555e-3
	gmf5 = 2.611088405080593625138020e-5
	gmf6 = 2.010321207683943062279931e-7
	gmf7 = 2.891024605872965461538222e-15
)

func gmErfinv(x float64) float64 {

	if gmIsNaN(x) || x <= -1 || x >= 1 {
		if x == -1 || x == 1 {
			return gmInf(int(x))
		}
		return gmNaN()
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}

	var ans float64
	if x <= 0.85 {
		r := 0.180625 - float64(float64(0.25*x)*x)
		z1 := float64((float64((float64((float64((float64((float64((float64(gma7*r)+gma6)*r)+gma5)*r)+gma4)*r)+gma3)*r)+gma2)*r)+gma1)*r) + gma0
		z2 := float64((float64((float64((float64((float64((float64((float64(gmb7*r)+gmb6)*r)+gmb5)*r)+gmb4)*r)+gmb3)*r)+gmb2)*r)+gmb1)*r) + gmb0
		ans = (float64(x * z1)) / z2
	} else {
		var z1, z2 float64
		r := gmSqrt(gmLn2 - gmLog(1.0-x))
		if r <= 5.0 {
			r -= 1.6
			z1 = float64((float64((float64((float64((float64((float64((float64(gmc7*r)+gmc6)*r)+gmc5)*r)+gmc4)*r)+gmc3)*r)+gmc2)*r)+gmc1)*r) + gmc0
			z2 = float64((float64((float64((float64((float64((float64((float64(gmd7*r)+gmd6)*r)+gmd5)*r)+gmd4)*r)+gmd3)*r)+gmd2)*r)+gmd1)*r) + gmd0
		} else {
			r -= 5.0
			z1 = float64((float64((float64((float64((float64((float64((float64(gme7*r)+gme6)*r)+gme5)*r)+gme4)*r)+gme3)*r)+gme2)*r)+gme1)*r) + gme0
			z2 = float64((float64((float64((float64((float64((float64((float64(gmf7*r)+gmf6)*r)+gmf5)*r)+gmf4)*r)+gmf3)*r)+gmf2)*r)+gmf1)*r) + gmf0
		}
		ans = z1 / z2
	}

	if sign {
		return -ans
	}
	return ans
}

func gmErfcinv(x float64) float64 {
	return gmErfinv(1 - x)
}

func gmExp(x float64) float64 {
	if gmhaveArchExp {
		return gmarchExp(x)
	}
	return gmexp(x)
}

func gmexp(x float64) float64 {
	const (
		Ln2Hi = 6.93147180369123816490e-01
		Ln2Lo = 1.90821492927058770002e-10
		Log2e = 1.44269504088896338700e+00

		Overflow  = 7.09782712893383973096e+02
		Underflow = -7.45133219101941108420e+02
		NearZero  = 1.0 / (1 << 28)
	)

	switch {
	case gmIsNaN(x):
		return x
	case x > Overflow:
		return gmInf(1)
	case x < Underflow:
		return 0
	case -NearZero < x && x < NearZero:
		return 1 + x
	}

	var k int
	switch {
	case x < 0:
		k = int(float64(Log2e*x) - 0.5)
	case x > 0:
		k = int(float64(Log2e*x) + 0.5)
	}
	hi := x - float64(float64(k)*Ln2Hi)
	lo := float64(float64(k) * Ln2Lo)

	return gmexpmulti(hi, lo, k)
}

func gmExp2(x float64) float64 {
	if gmhaveArchExp2 {
		return gmarchExp2(x)
	}
	return gmexp2(x)
}

func gmexp2(x float64) float64 {
	const (
		Ln2Hi = 6.93147180369123816490e-01
		Ln2Lo = 1.90821492927058770002e-10

		Overflow  = 1.0239999999999999e+03
		Underflow = -1.0740e+03
	)

	switch {
	case gmIsNaN(x):
		return x
	case x > Overflow:
		return gmInf(1)
	case x < Underflow:
		return 0
	}

	var k int
	switch {
	case x > 0:
		k = int(x + 0.5)
	case x < 0:
		k = int(x - 0.5)
	}
	t := x - float64(k)
	hi := float64(t * Ln2Hi)
	lo := float64(-t * Ln2Lo)

	return gmexpmulti(hi, lo, k)
}

func gmexpmulti(hi, lo float64, k int) float64 {
	const (
		P1 = 1.66666666666666657415e-01
		P2 = -2.77777777770155933842e-03
		P3 = 6.61375632143793436117e-05
		P4 = -1.65339022054652515390e-06
		P5 = 4.13813679705723846039e-08
	)

	r := hi - lo
	t := float64(r * r)
	c := r - float64(t*(P1+float64(t*(P2+float64(t*(P3+float64(t*(P4+float64(t*P5)))))))))
	y := 1 - ((lo - (float64(r*c))/(2-c)) - hi)

	return gmLdexp(y, k)
}

const gmhaveArchExp2 = false

func gmarchExp2(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchExp = false

func gmarchExp(x float64) float64 {
	panic("not implemented")
}

func gmExpm1(x float64) float64 {
	if gmhaveArchExpm1 {
		return gmarchExpm1(x)
	}
	return gmexpm1(x)
}

func gmexpm1(x float64) float64 {
	const (
		Othreshold = 7.09782712893383973096e+02
		Ln2X56     = 3.88162421113569373274e+01
		Ln2HalfX3  = 1.03972077083991796413e+00
		Ln2Half    = 3.46573590279972654709e-01
		Ln2Hi      = 6.93147180369123816490e-01
		Ln2Lo      = 1.90821492927058770002e-10
		InvLn2     = 1.44269504088896338700e+00
		Tiny       = 1.0 / (1 << 54)

		Q1 = -3.33333333333331316428e-02
		Q2 = 1.58730158725481460165e-03
		Q3 = -7.93650757867487942473e-05
		Q4 = 4.00821782732936239552e-06
		Q5 = -2.01099218183624371326e-07
	)

	switch {
	case gmIsInf(x, 1) || gmIsNaN(x):
		return x
	case gmIsInf(x, -1):
		return -1
	}

	absx := x
	sign := false
	if x < 0 {
		absx = -absx
		sign = true
	}

	if absx >= Ln2X56 {
		if sign {
			return -1
		}
		if absx >= Othreshold {
			return gmInf(1)
		}
	}

	var c float64
	var k int
	if absx > Ln2Half {
		var hi, lo float64
		if absx < Ln2HalfX3 {
			if !sign {
				hi = x - Ln2Hi
				lo = Ln2Lo
				k = 1
			} else {
				hi = x + Ln2Hi
				lo = -Ln2Lo
				k = -1
			}
		} else {
			if !sign {
				k = int(float64(InvLn2*x) + 0.5)
			} else {
				k = int(float64(InvLn2*x) - 0.5)
			}
			t := float64(k)
			hi = x - float64(t*Ln2Hi)
			lo = float64(t * Ln2Lo)
		}
		x = hi - lo
		c = (hi - x) - lo
	} else if absx < Tiny {
		return x
	} else {
		k = 0
	}

	hfx := float64(0.5 * x)
	hxs := float64(x * hfx)
	r1 := 1 + float64(hxs*(Q1+float64(hxs*(Q2+float64(hxs*(Q3+float64(hxs*(Q4+float64(hxs*Q5)))))))))
	t := 3 - float64(r1*hfx)
	e := float64(hxs * ((r1 - t) / (6.0 - float64(x*t))))
	if k == 0 {
		return x - (float64(x*e) - hxs)
	}
	e = (float64(x*(e-c)) - c)
	e -= hxs
	switch {
	case k == -1:
		return float64(0.5*(x-e)) - 0.5
	case k == 1:
		if x < -0.25 {
			return float64(-2 * (e - (x + 0.5)))
		}
		return 1 + float64(2*(x-e))
	case k <= -2 || k > 56:
		y := 1 - (e - x)
		y = gmFloat64frombits(gmFloat64bits(y) + uint64(k)<<52)
		return y - 1
	}
	if k < 20 {
		t := gmFloat64frombits(0x3ff0000000000000 - (0x20000000000000 >> uint(k)))
		y := t - (e - x)
		y = gmFloat64frombits(gmFloat64bits(y) + uint64(k)<<52)
		return y
	}
	t = gmFloat64frombits(uint64(0x3ff-k) << 52)
	y := x - (e + t)
	y++
	y = gmFloat64frombits(gmFloat64bits(y) + uint64(k)<<52)
	return y
}

func gmFloor(x float64) float64 {
	if gmhaveArchFloor {
		return gmarchFloor(x)
	}
	return gmfloor(x)
}

func gmfloor(x float64) float64 {
	if x == 0 || gmIsNaN(x) || gmIsInf(x, 0) {
		return x
	}
	if x < 0 {
		d, fract := gmModf(-x)
		if fract != 0.0 {
			d = d + 1
		}
		return -d
	}
	d, _ := gmModf(x)
	return d
}

func gmCeil(x float64) float64 {
	if gmhaveArchCeil {
		return gmarchCeil(x)
	}
	return gmceil(x)
}

func gmceil(x float64) float64 {
	return -gmFloor(-x)
}

func gmTrunc(x float64) float64 {
	if gmhaveArchTrunc {
		return gmarchTrunc(x)
	}
	return gmtrunc(x)
}

func gmtrunc(x float64) float64 {
	if gmAbs(x) < 1 {
		return gmCopysign(0, x)
	}

	b := gmFloat64bits(x)
	e := uint(b>>gmshift)&gmmask - gmbias

	if e < 64-12 {
		b &^= 1<<(64-12-e) - 1
	}
	return gmFloat64frombits(b)
}

func gmRound(x float64) float64 {

	bits := gmFloat64bits(x)
	e := uint(bits>>gmshift) & gmmask
	if e < gmbias {

		bits &= gmsignMask
		if e == gmbias-1 {
			bits |= gmuvone
		}
	} else if e < gmbias+gmshift {

		const half = 1 << (gmshift - 1)
		e -= gmbias
		bits += half >> e
		bits &^= gmfracMask >> e
	}
	return gmFloat64frombits(bits)
}

func gmRoundToEven(x float64) float64 {

	bits := gmFloat64bits(x)
	e := uint(bits>>gmshift) & gmmask
	if e >= gmbias {

		const halfMinusULP = (1 << (gmshift - 1)) - 1
		e -= gmbias
		bits += (halfMinusULP + (bits>>(gmshift-e))&1) >> e
		bits &^= gmfracMask >> e
	} else if e == gmbias-1 && bits&gmfracMask != 0 {

		bits = bits&gmsignMask | gmuvone
	} else {

		bits &= gmsignMask
	}
	return gmFloat64frombits(bits)
}

const gmhaveArchFloor = false

func gmarchFloor(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchCeil = false

func gmarchCeil(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchTrunc = false

func gmarchTrunc(x float64) float64 {
	panic("not implemented")
}

func gmzero(x uint64) uint64 {
	if x == 0 {
		return 1
	}
	return 0

}

func gmnonzero(x uint64) uint64 {
	if x != 0 {
		return 1
	}
	return 0

}

func gmshl(u1, u2 uint64, n uint) (r1, r2 uint64) {
	r1 = u1<<n | u2>>(64-n) | u2<<(n-64)
	r2 = u2 << n
	return
}

func gmshr(u1, u2 uint64, n uint) (r1, r2 uint64) {
	r2 = u2>>n | u1<<(64-n) | u1>>(n-64)
	r1 = u1 >> n
	return
}

func gmshrcompress(u1, u2 uint64, n uint) (r1, r2 uint64) {

	switch {
	case n == 0:
		return u1, u2
	case n == 64:
		return 0, u1 | gmnonzero(u2)
	case n >= 128:
		return 0, gmnonzero(u1 | u2)
	case n < 64:
		r1, r2 = gmshr(u1, u2, n)
		r2 |= gmnonzero(u2 & (1<<n - 1))
	case n < 128:
		r1, r2 = gmshr(u1, u2, n)
		r2 |= gmnonzero(u1&(1<<(n-64)-1) | u2)
	}
	return
}

func gmlz(u1, u2 uint64) (l int32) {
	l = int32(bits.LeadingZeros64(u1))
	if l == 64 {
		l += int32(bits.LeadingZeros64(u2))
	}
	return l
}

func gmsplit(b uint64) (sign uint32, exp int32, mantissa uint64) {
	sign = uint32(b >> 63)
	exp = int32(b>>52) & gmmask
	mantissa = b & gmfracMask

	if exp == 0 {

		shift := uint(bits.LeadingZeros64(mantissa) - 11)
		mantissa <<= shift
		exp = 1 - int32(shift)
	} else {

		mantissa |= 1 << 52
	}
	return
}

func gmFMA(x, y, z float64) float64 {
	bx, by, bz := gmFloat64bits(x), gmFloat64bits(y), gmFloat64bits(z)

	if x == 0.0 || y == 0.0 || bx&gmuvinf == gmuvinf || by&gmuvinf == gmuvinf {
		return float64(x*y) + z
	}

	if z == 0.0 {
		return float64(x * y)
	}

	if bz&gmuvinf == gmuvinf {
		return z
	}

	xs, xe, xm := gmsplit(bx)
	ys, ye, ym := gmsplit(by)
	zs, ze, zm := gmsplit(bz)

	pe := xe + ye - gmbias + 1

	pm1, pm2 := bits.Mul64(xm<<10, ym<<11)
	zm1, zm2 := zm<<10, uint64(0)
	ps := xs ^ ys

	is62zero := uint((^pm1 >> 62) & 1)
	pm1, pm2 = gmshl(pm1, pm2, is62zero)
	pe -= int32(is62zero)

	if pe < ze || pe == ze && pm1 < zm1 {
		ps, pe, pm1, pm2, zs, ze, zm1, zm2 = zs, ze, zm1, zm2, ps, pe, pm1, pm2
	}

	if ps != zs && pe == ze && pm1 == zm1 && pm2 == zm2 {
		return 0
	}

	zm1, zm2 = gmshrcompress(zm1, zm2, uint(pe-ze))

	var m, c uint64
	if ps == zs {

		pm2, c = bits.Add64(pm2, zm2, 0)
		pm1, _ = bits.Add64(pm1, zm1, c)
		pe -= int32(^pm1 >> 63)
		pm1, m = gmshrcompress(pm1, pm2, uint(64+pm1>>63))
	} else {

		pm2, c = bits.Sub64(pm2, zm2, 0)
		pm1, _ = bits.Sub64(pm1, zm1, c)
		nz := gmlz(pm1, pm2)
		pe -= nz
		m, pm2 = gmshl(pm1, pm2, uint(nz-1))
		m |= gmnonzero(pm2)
	}

	if pe > 1022+gmbias || pe == 1022+gmbias && (m+1<<9)>>63 == 1 {

		return gmFloat64frombits(uint64(ps)<<63 | gmuvinf)
	}
	if pe < 0 {
		n := uint(-pe)
		m = m>>n | gmnonzero(m&(1<<n-1))
		pe = 0
	}
	m = ((m + 1<<9) >> 10) & ^gmzero((m&(1<<10-1))^1<<9)
	pe &= -int32(gmnonzero(m))
	return gmFloat64frombits(uint64(ps)<<63 + uint64(pe)<<52 + m)
}

func gmFrexp(f float64) (frac float64, exp int) {
	if gmhaveArchFrexp {
		return gmarchFrexp(f)
	}
	return gmfrexp(f)
}

func gmfrexp(f float64) (frac float64, exp int) {

	switch {
	case f == 0:
		return f, 0
	case gmIsInf(f, 0) || gmIsNaN(f):
		return f, 0
	}
	f, exp = gmnormalize(f)
	x := gmFloat64bits(f)
	exp += int((x>>gmshift)&gmmask) - gmbias + 1
	x &^= gmmask << gmshift
	x |= (-1 + gmbias) << gmshift
	frac = gmFloat64frombits(x)
	return
}

var gm_gamP = [...]float64{
	1.60119522476751861407e-04,
	1.19135147006586384913e-03,
	1.04213797561761569935e-02,
	4.76367800457137231464e-02,
	2.07448227648435975150e-01,
	4.94214826801497100753e-01,
	9.99999999999999996796e-01,
}

var gm_gamQ = [...]float64{
	-2.31581873324120129819e-05,
	5.39605580493303397842e-04,
	-4.45641913851797240494e-03,
	1.18139785222060435552e-02,
	3.58236398605498653373e-02,
	-2.34591795718243348568e-01,
	7.14304917030273074085e-02,
	1.00000000000000000320e+00,
}

var gm_gamS = [...]float64{
	7.87311395793093628397e-04,
	-2.29549961613378126380e-04,
	-2.68132617805781232825e-03,
	3.47222221605458667310e-03,
	8.33333333333482257126e-02,
}

func gmstirling(x float64) (float64, float64) {
	if x > 200 {
		return gmInf(1), 1
	}
	const (
		SqrtTwoPi   = 2.506628274631000502417
		MaxStirling = 143.01608
	)
	w := 1 / x
	w = 1 + float64(w*(float64((float64((float64((float64(gm_gamS[0]*w)+gm_gamS[1])*w)+gm_gamS[2])*w)+gm_gamS[3])*w)+gm_gamS[4]))
	y1 := gmExp(x)
	y2 := 1.0
	if x > MaxStirling {
		v := gmPow(x, float64(0.5*x)-0.25)
		y1, y2 = v, v/y1
	} else {
		y1 = gmPow(x, x-0.5) / y1
	}
	return y1, float64(float64(SqrtTwoPi*w) * y2)
}

func gmGamma(x float64) float64 {
	const Euler = 0.57721566490153286060651209008240243104215933593992

	switch {
	case gmisNegInt(x) || gmIsInf(x, -1) || gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return gmInf(1)
	case x == 0:
		if gmSignbit(x) {
			return gmInf(-1)
		}
		return gmInf(1)
	}
	q := gmAbs(x)
	p := gmFloor(q)
	if q > 33 {
		if x >= 0 {
			y1, y2 := gmstirling(x)
			return float64(y1 * y2)
		}

		signgam := 1
		if ip := int64(p); ip&1 == 0 {
			signgam = -1
		}
		z := q - p
		if z > 0.5 {
			p = p + 1
			z = q - p
		}
		z = float64(q * gmSin(float64(gmPi*z)))
		if z == 0 {
			return gmInf(signgam)
		}
		sq1, sq2 := gmstirling(q)
		absz := gmAbs(z)
		d := float64(float64(absz*sq1) * sq2)
		if gmIsInf(d, 0) {
			z = gmPi / absz / sq1 / sq2
		} else {
			z = gmPi / d
		}
		return float64(float64(signgam) * z)
	}

	z := 1.0
	for x >= 3 {
		x = x - 1
		z = float64(z * x)
	}
	for x < 0 {
		if x > -1e-09 {
			goto small
		}
		z = z / x
		x = x + 1
	}
	for x < 2 {
		if x < 1e-09 {
			goto small
		}
		z = z / x
		x = x + 1
	}

	if x == 2 {
		return z
	}

	x = x - 2
	p = float64((float64((float64((float64((float64((float64(x*gm_gamP[0])+gm_gamP[1])*x)+gm_gamP[2])*x)+gm_gamP[3])*x)+gm_gamP[4])*x)+gm_gamP[5])*x) + gm_gamP[6]
	q = float64((float64((float64((float64((float64((float64((float64(x*gm_gamQ[0])+gm_gamQ[1])*x)+gm_gamQ[2])*x)+gm_gamQ[3])*x)+gm_gamQ[4])*x)+gm_gamQ[5])*x)+gm_gamQ[6])*x) + gm_gamQ[7]
	return float64(z*p) / q

small:
	if x == 0 {
		return gmInf(1)
	}
	return z / (float64((1 + float64(Euler*x)) * x))
}

func gmisNegInt(x float64) bool {
	if x < 0 {
		_, xf := gmModf(x)
		return xf == 0
	}
	return false
}

func gmHypot(p, q float64) float64 {
	if gmhaveArchHypot {
		return gmarchHypot(p, q)
	}
	return gmhypot(p, q)
}

func gmhypot(p, q float64) float64 {
	p, q = gmAbs(p), gmAbs(q)

	switch {
	case gmIsInf(p, 1) || gmIsInf(q, 1):
		return gmInf(1)
	case gmIsNaN(p) || gmIsNaN(q):
		return gmNaN()
	}
	if p < q {
		p, q = q, p
	}
	if p == 0 {
		return 0
	}
	q = q / p
	return float64(p * gmSqrt(1+float64(q*q)))
}

const gmhaveArchHypot = false

func gmarchHypot(p, q float64) float64 {
	panic("not implemented")
}

func gmJ0(x float64) float64 {
	const (
		Huge   = 1e300
		TwoM27 = 1.0 / (1 << 27)
		TwoM13 = 1.0 / (1 << 13)
		Two129 = 1 << 129

		R02 = 1.56249999999999947958e-02
		R03 = -1.89979294238854721751e-04
		R04 = 1.82954049532700665670e-06
		R05 = -4.61832688532103189199e-09
		S01 = 1.56191029464890010492e-02
		S02 = 1.16926784663337450260e-04
		S03 = 5.13546550207318111446e-07
		S04 = 1.16614003333790000205e-09
	)

	switch {
	case gmIsNaN(x):
		return x
	case gmIsInf(x, 0):
		return 0
	case x == 0:
		return 1
	}

	x = gmAbs(x)
	if x >= 2 {
		s, c := gmSincos(x)
		ss := s - c
		cc := s + c

		if x < gmMaxFloat64/2 {
			z := -gmCos(x + x)
			if float64(s*c) < 0 {
				cc = z / ss
			} else {
				ss = z / cc
			}
		}

		var z float64
		if x > Two129 {
			z = float64((1/gmSqrtPi)*cc) / gmSqrt(x)
		} else {
			u := gmpzero(x)
			v := gmqzero(x)
			z = float64((1/gmSqrtPi)*(float64(u*cc)-float64(v*ss))) / gmSqrt(x)
		}
		return z
	}
	if x < TwoM13 {
		if x < TwoM27 {
			return 1
		}
		return 1 - float64(float64(0.25*x)*x)
	}
	z := float64(x * x)
	r := float64(z * (R02 + float64(z*(R03+float64(z*(R04+float64(z*R05)))))))
	s := 1 + float64(z*(S01+float64(z*(S02+float64(z*(S03+float64(z*S04)))))))
	if x < 1 {
		return 1 + float64(z*(-0.25+(r/s)))
	}
	u := float64(0.5 * x)
	return float64((1+u)*(1-u)) + float64(z*(r/s))
}

func gmY0(x float64) float64 {
	const (
		TwoM27 = 1.0 / (1 << 27)
		Two129 = 1 << 129
		U00    = -7.38042951086872317523e-02
		U01    = 1.76666452509181115538e-01
		U02    = -1.38185671945596898896e-02
		U03    = 3.47453432093683650238e-04
		U04    = -3.81407053724364161125e-06
		U05    = 1.95590137035022920206e-08
		U06    = -3.98205194132103398453e-11
		V01    = 1.27304834834123699328e-02
		V02    = 7.60068627350353253702e-05
		V03    = 2.59150851840457805467e-07
		V04    = 4.41110311332675467403e-10
	)

	switch {
	case x < 0 || gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return 0
	case x == 0:
		return gmInf(-1)
	}

	if x >= 2 {

		s, c := gmSincos(x)
		ss := s - c
		cc := s + c

		if x < gmMaxFloat64/2 {
			z := -gmCos(x + x)
			if float64(s*c) < 0 {
				cc = z / ss
			} else {
				ss = z / cc
			}
		}
		var z float64
		if x > Two129 {
			z = float64((1/gmSqrtPi)*ss) / gmSqrt(x)
		} else {
			u := gmpzero(x)
			v := gmqzero(x)
			z = float64((1/gmSqrtPi)*(float64(u*ss)+float64(v*cc))) / gmSqrt(x)
		}
		return z
	}
	if x <= TwoM27 {
		return U00 + float64((2/gmPi)*gmLog(x))
	}
	z := float64(x * x)
	u := U00 + float64(z*(U01+float64(z*(U02+float64(z*(U03+float64(z*(U04+float64(z*(U05+float64(z*U06)))))))))))
	v := 1 + float64(z*(V01+float64(z*(V02+float64(z*(V03+float64(z*V04)))))))
	return u/v + float64(float64((2/gmPi)*gmJ0(x))*gmLog(x))
}

var gmp0R8 = [6]float64{
	0.00000000000000000000e+00,
	-7.03124999999900357484e-02,
	-8.08167041275349795626e+00,
	-2.57063105679704847262e+02,
	-2.48521641009428822144e+03,
	-5.25304380490729545272e+03,
}

var gmp0S8 = [5]float64{
	1.16534364619668181717e+02,
	3.83374475364121826715e+03,
	4.05978572648472545552e+04,
	1.16752972564375915681e+05,
	4.76277284146730962675e+04,
}

var gmp0R5 = [6]float64{
	-1.14125464691894502584e-11,
	-7.03124940873599280078e-02,
	-4.15961064470587782438e+00,
	-6.76747652265167261021e+01,
	-3.31231299649172967747e+02,
	-3.46433388365604912451e+02,
}

var gmp0S5 = [5]float64{
	6.07539382692300335975e+01,
	1.05125230595704579173e+03,
	5.97897094333855784498e+03,
	9.62544514357774460223e+03,
	2.40605815922939109441e+03,
}

var gmp0R3 = [6]float64{
	-2.54704601771951915620e-09,
	-7.03119616381481654654e-02,
	-2.40903221549529611423e+00,
	-2.19659774734883086467e+01,
	-5.80791704701737572236e+01,
	-3.14479470594888503854e+01,
}

var gmp0S3 = [5]float64{
	3.58560338055209726349e+01,
	3.61513983050303863820e+02,
	1.19360783792111533330e+03,
	1.12799679856907414432e+03,
	1.73580930813335754692e+02,
}

var gmp0R2 = [6]float64{
	-8.87534333032526411254e-08,
	-7.03030995483624743247e-02,
	-1.45073846780952986357e+00,
	-7.63569613823527770791e+00,
	-1.11931668860356747786e+01,
	-3.23364579351335335033e+00,
}

var gmp0S2 = [5]float64{
	2.22202997532088808441e+01,
	1.36206794218215208048e+02,
	2.70470278658083486789e+02,
	1.53875394208320329881e+02,
	1.46576176948256193810e+01,
}

func gmpzero(x float64) float64 {
	var p *[6]float64
	var q *[5]float64
	if x >= 8 {
		p = &gmp0R8
		q = &gmp0S8
	} else if x >= 4.5454 {
		p = &gmp0R5
		q = &gmp0S5
	} else if x >= 2.8571 {
		p = &gmp0R3
		q = &gmp0S3
	} else if x >= 2 {
		p = &gmp0R2
		q = &gmp0S2
	}
	z := 1 / (float64(x * x))
	r := p[0] + float64(z*(p[1]+float64(z*(p[2]+float64(z*(p[3]+float64(z*(p[4]+float64(z*p[5])))))))))
	s := 1 + float64(z*(q[0]+float64(z*(q[1]+float64(z*(q[2]+float64(z*(q[3]+float64(z*q[4])))))))))
	return 1 + r/s
}

var gmq0R8 = [6]float64{
	0.00000000000000000000e+00,
	7.32421874999935051953e-02,
	1.17682064682252693899e+01,
	5.57673380256401856059e+02,
	8.85919720756468632317e+03,
	3.70146267776887834771e+04,
}

var gmq0S8 = [6]float64{
	1.63776026895689824414e+02,
	8.09834494656449805916e+03,
	1.42538291419120476348e+05,
	8.03309257119514397345e+05,
	8.40501579819060512818e+05,
	-3.43899293537866615225e+05,
}

var gmq0R5 = [6]float64{
	1.84085963594515531381e-11,
	7.32421766612684765896e-02,
	5.83563508962056953777e+00,
	1.35111577286449829671e+02,
	1.02724376596164097464e+03,
	1.98997785864605384631e+03,
}

var gmq0S5 = [6]float64{
	8.27766102236537761883e+01,
	2.07781416421392987104e+03,
	1.88472887785718085070e+04,
	5.67511122894947329769e+04,
	3.59767538425114471465e+04,
	-5.35434275601944773371e+03,
}

var gmq0R3 = [6]float64{
	4.37741014089738620906e-09,
	7.32411180042911447163e-02,
	3.34423137516170720929e+00,
	4.26218440745412650017e+01,
	1.70808091340565596283e+02,
	1.66733948696651168575e+02,
}

var gmq0S3 = [6]float64{
	4.87588729724587182091e+01,
	7.09689221056606015736e+02,
	3.70414822620111362994e+03,
	6.46042516752568917582e+03,
	2.51633368920368957333e+03,
	-1.49247451836156386662e+02,
}

var gmq0R2 = [6]float64{
	1.50444444886983272379e-07,
	7.32234265963079278272e-02,
	1.99819174093815998816e+00,
	1.44956029347885735348e+01,
	3.16662317504781540833e+01,
	1.62527075710929267416e+01,
}

var gmq0S2 = [6]float64{
	3.03655848355219184498e+01,
	2.69348118608049844624e+02,
	8.44783757595320139444e+02,
	8.82935845112488550512e+02,
	2.12666388511798828631e+02,
	-5.31095493882666946917e+00,
}

func gmqzero(x float64) float64 {
	var p, q *[6]float64
	if x >= 8 {
		p = &gmq0R8
		q = &gmq0S8
	} else if x >= 4.5454 {
		p = &gmq0R5
		q = &gmq0S5
	} else if x >= 2.8571 {
		p = &gmq0R3
		q = &gmq0S3
	} else if x >= 2 {
		p = &gmq0R2
		q = &gmq0S2
	}
	z := 1 / (float64(x * x))
	r := p[0] + float64(z*(p[1]+float64(z*(p[2]+float64(z*(p[3]+float64(z*(p[4]+float64(z*p[5])))))))))
	s := 1 + float64(z*(q[0]+float64(z*(q[1]+float64(z*(q[2]+float64(z*(q[3]+float64(z*(q[4]+float64(z*q[5])))))))))))
	return (-0.125 + r/s) / x
}

func gmJ1(x float64) float64 {
	const (
		TwoM27 = 1.0 / (1 << 27)
		Two129 = 1 << 129

		R00 = -6.25000000000000000000e-02
		R01 = 1.40705666955189706048e-03
		R02 = -1.59955631084035597520e-05
		R03 = 4.96727999609584448412e-08
		S01 = 1.91537599538363460805e-02
		S02 = 1.85946785588630915560e-04
		S03 = 1.17718464042623683263e-06
		S04 = 5.04636257076217042715e-09
		S05 = 1.23542274426137913908e-11
	)

	switch {
	case gmIsNaN(x):
		return x
	case gmIsInf(x, 0) || x == 0:
		return 0
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if x >= 2 {
		s, c := gmSincos(x)
		ss := -s - c
		cc := s - c

		if x < gmMaxFloat64/2 {
			z := gmCos(x + x)
			if float64(s*c) > 0 {
				cc = z / ss
			} else {
				ss = z / cc
			}
		}

		var z float64
		if x > Two129 {
			z = float64((1/gmSqrtPi)*cc) / gmSqrt(x)
		} else {
			u := gmpone(x)
			v := gmqone(x)
			z = float64((1/gmSqrtPi)*(float64(u*cc)-float64(v*ss))) / gmSqrt(x)
		}
		if sign {
			return -z
		}
		return z
	}
	if x < TwoM27 {
		return float64(0.5 * x)
	}
	z := float64(x * x)
	r := float64(z * (R00 + float64(z*(R01+float64(z*(R02+float64(z*R03)))))))
	s := 1.0 + float64(z*(S01+float64(z*(S02+float64(z*(S03+float64(z*(S04+float64(z*S05)))))))))
	r *= x
	z = float64(0.5*x) + r/s
	if sign {
		return -z
	}
	return z
}

func gmY1(x float64) float64 {
	const (
		TwoM54 = 1.0 / (1 << 54)
		Two129 = 1 << 129
		U00    = -1.96057090646238940668e-01
		U01    = 5.04438716639811282616e-02
		U02    = -1.91256895875763547298e-03
		U03    = 2.35252600561610495928e-05
		U04    = -9.19099158039878874504e-08
		V00    = 1.99167318236649903973e-02
		V01    = 2.02552581025135171496e-04
		V02    = 1.35608801097516229404e-06
		V03    = 6.22741452364621501295e-09
		V04    = 1.66559246207992079114e-11
	)

	switch {
	case x < 0 || gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return 0
	case x == 0:
		return gmInf(-1)
	}

	if x >= 2 {
		s, c := gmSincos(x)
		ss := -s - c
		cc := s - c

		if x < gmMaxFloat64/2 {
			z := gmCos(x + x)
			if float64(s*c) > 0 {
				cc = z / ss
			} else {
				ss = z / cc
			}
		}

		var z float64
		if x > Two129 {
			z = float64((1/gmSqrtPi)*ss) / gmSqrt(x)
		} else {
			u := gmpone(x)
			v := gmqone(x)
			z = float64((1/gmSqrtPi)*(float64(u*ss)+float64(v*cc))) / gmSqrt(x)
		}
		return z
	}
	if x <= TwoM54 {
		return -(2 / gmPi) / x
	}
	z := float64(x * x)
	u := U00 + float64(z*(U01+float64(z*(U02+float64(z*(U03+float64(z*U04)))))))
	v := 1 + float64(z*(V00+float64(z*(V01+float64(z*(V02+float64(z*(V03+float64(z*V04)))))))))
	return float64(x*(u/v)) + float64((2/gmPi)*(float64(gmJ1(x)*gmLog(x))-1/x))
}

var gmp1R8 = [6]float64{
	0.00000000000000000000e+00,
	1.17187499999988647970e-01,
	1.32394806593073575129e+01,
	4.12051854307378562225e+02,
	3.87474538913960532227e+03,
	7.91447954031891731574e+03,
}

var gmp1S8 = [5]float64{
	1.14207370375678408436e+02,
	3.65093083420853463394e+03,
	3.69562060269033463555e+04,
	9.76027935934950801311e+04,
	3.08042720627888811578e+04,
}

var gmp1R5 = [6]float64{
	1.31990519556243522749e-11,
	1.17187493190614097638e-01,
	6.80275127868432871736e+00,
	1.08308182990189109773e+02,
	5.17636139533199752805e+02,
	5.28715201363337541807e+02,
}

var gmp1S5 = [5]float64{
	5.92805987221131331921e+01,
	9.91401418733614377743e+02,
	5.35326695291487976647e+03,
	7.84469031749551231769e+03,
	1.50404688810361062679e+03,
}

var gmp1R3 = [6]float64{
	3.02503916137373618024e-09,
	1.17186865567253592491e-01,
	3.93297750033315640650e+00,
	3.51194035591636932736e+01,
	9.10550110750781271918e+01,
	4.85590685197364919645e+01,
}

var gmp1S3 = [5]float64{
	3.47913095001251519989e+01,
	3.36762458747825746741e+02,
	1.04687139975775130551e+03,
	8.90811346398256432622e+02,
	1.03787932439639277504e+02,
}

var gmp1R2 = [6]float64{
	1.07710830106873743082e-07,
	1.17176219462683348094e-01,
	2.36851496667608785174e+00,
	1.22426109148261232917e+01,
	1.76939711271687727390e+01,
	5.07352312588818499250e+00,
}

var gmp1S2 = [5]float64{
	2.14364859363821409488e+01,
	1.25290227168402751090e+02,
	2.32276469057162813669e+02,
	1.17679373287147100768e+02,
	8.36463893371618283368e+00,
}

func gmpone(x float64) float64 {
	var p *[6]float64
	var q *[5]float64
	if x >= 8 {
		p = &gmp1R8
		q = &gmp1S8
	} else if x >= 4.5454 {
		p = &gmp1R5
		q = &gmp1S5
	} else if x >= 2.8571 {
		p = &gmp1R3
		q = &gmp1S3
	} else if x >= 2 {
		p = &gmp1R2
		q = &gmp1S2
	}
	z := 1 / (float64(x * x))
	r := p[0] + float64(z*(p[1]+float64(z*(p[2]+float64(z*(p[3]+float64(z*(p[4]+float64(z*p[5])))))))))
	s := 1.0 + float64(z*(q[0]+float64(z*(q[1]+float64(z*(q[2]+float64(z*(q[3]+float64(z*q[4])))))))))
	return 1 + r/s
}

var gmq1R8 = [6]float64{
	0.00000000000000000000e+00,
	-1.02539062499992714161e-01,
	-1.62717534544589987888e+01,
	-7.59601722513950107896e+02,
	-1.18498066702429587167e+04,
	-4.84385124285750353010e+04,
}

var gmq1S8 = [6]float64{
	1.61395369700722909556e+02,
	7.82538599923348465381e+03,
	1.33875336287249578163e+05,
	7.19657723683240939863e+05,
	6.66601232617776375264e+05,
	-2.94490264303834643215e+05,
}

var gmq1R5 = [6]float64{
	-2.08979931141764104297e-11,
	-1.02539050241375426231e-01,
	-8.05644828123936029840e+00,
	-1.83669607474888380239e+02,
	-1.37319376065508163265e+03,
	-2.61244440453215656817e+03,
}

var gmq1S5 = [6]float64{
	8.12765501384335777857e+01,
	1.99179873460485964642e+03,
	1.74684851924908907677e+04,
	4.98514270910352279316e+04,
	2.79480751638918118260e+04,
	-4.71918354795128470869e+03,
}

var gmq1R3 = [6]float64{
	-5.07831226461766561369e-09,
	-1.02537829820837089745e-01,
	-4.61011581139473403113e+00,
	-5.78472216562783643212e+01,
	-2.28244540737631695038e+02,
	-2.19210128478909325622e+02,
}

var gmq1S3 = [6]float64{
	4.76651550323729509273e+01,
	6.73865112676699709482e+02,
	3.38015286679526343505e+03,
	5.54772909720722782367e+03,
	1.90311919338810798763e+03,
	-1.35201191444307340817e+02,
}

var gmq1R2 = [6]float64{
	-1.78381727510958865572e-07,
	-1.02517042607985553460e-01,
	-2.75220568278187460720e+00,
	-1.96636162643703720221e+01,
	-4.23253133372830490089e+01,
	-2.13719211703704061733e+01,
}

var gmq1S2 = [6]float64{
	2.95333629060523854548e+01,
	2.52981549982190529136e+02,
	7.57502834868645436472e+02,
	7.39393205320467245656e+02,
	1.55949003336666123687e+02,
	-4.95949898822628210127e+00,
}

func gmqone(x float64) float64 {
	var p, q *[6]float64
	if x >= 8 {
		p = &gmq1R8
		q = &gmq1S8
	} else if x >= 4.5454 {
		p = &gmq1R5
		q = &gmq1S5
	} else if x >= 2.8571 {
		p = &gmq1R3
		q = &gmq1S3
	} else if x >= 2 {
		p = &gmq1R2
		q = &gmq1S2
	}
	z := 1 / (float64(x * x))
	r := p[0] + float64(z*(p[1]+float64(z*(p[2]+float64(z*(p[3]+float64(z*(p[4]+float64(z*p[5])))))))))
	s := 1 + float64(z*(q[0]+float64(z*(q[1]+float64(z*(q[2]+float64(z*(q[3]+float64(z*(q[4]+float64(z*q[5])))))))))))
	return (0.375 + r/s) / x
}

func gmJn(n int, x float64) float64 {
	const (
		TwoM29 = 1.0 / (1 << 29)
		Two302 = 1 << 302
	)

	switch {
	case gmIsNaN(x):
		return x
	case gmIsInf(x, 0):
		return 0
	}

	if n == 0 {
		return gmJ0(x)
	}
	if x == 0 {
		return 0
	}
	if n < 0 {
		n, x = -n, -x
	}
	if n == 1 {
		return gmJ1(x)
	}
	sign := false
	if x < 0 {
		x = -x
		if n&1 == 1 {
			sign = true
		}
	}
	var b float64
	if float64(n) <= x {

		if x >= Two302 {

			var temp float64
			switch s, c := gmSincos(x); n & 3 {
			case 0:
				temp = c + s
			case 1:
				temp = -c + s
			case 2:
				temp = -c - s
			case 3:
				temp = c - s
			}
			b = float64((1/gmSqrtPi)*temp) / gmSqrt(x)
		} else {
			b = gmJ1(x)
			for i, a := 1, gmJ0(x); i < n; i++ {
				a, b = b, float64(b*(float64(i+i)/x))-a
			}
		}
	} else {
		if x < TwoM29 {

			if n > 33 {
				b = 0
			} else {
				temp := float64(x * 0.5)
				b = temp
				a := 1.0
				for i := 2; i <= n; i++ {
					a *= float64(i)
					b *= temp
				}
				b /= a
			}
		} else {

			w := float64(n+n) / x
			h := 2 / x
			q0 := w
			z := w + h
			q1 := float64(w*z) - 1
			k := 1
			for q1 < 1e9 {
				k++
				z += h
				q0, q1 = q1, float64(z*q1)-q0
			}
			m := n + n
			t := 0.0
			for i := 2 * (n + k); i >= m; i -= 2 {
				t = 1 / (float64(i)/x - t)
			}
			a := t
			b = 1

			tmp := float64(n)
			v := 2 / x
			tmp = float64(tmp * gmLog(gmAbs(float64(v*tmp))))
			if tmp < 7.09782712893383973096e+02 {
				for i := n - 1; i > 0; i-- {
					di := float64(i + i)
					a, b = b, float64(b*di)/x-a
				}
			} else {
				for i := n - 1; i > 0; i-- {
					di := float64(i + i)
					a, b = b, float64(b*di)/x-a

					if b > 1e100 {
						a /= b
						t /= b
						b = 1
					}
				}
			}
			b = float64(t*gmJ0(x)) / b
		}
	}
	if sign {
		return -b
	}
	return b
}

func gmYn(n int, x float64) float64 {
	const Two302 = 1 << 302

	switch {
	case x < 0 || gmIsNaN(x):
		return gmNaN()
	case gmIsInf(x, 1):
		return 0
	}

	if n == 0 {
		return gmY0(x)
	}
	if x == 0 {
		if n < 0 && n&1 == 1 {
			return gmInf(1)
		}
		return gmInf(-1)
	}
	sign := false
	if n < 0 {
		n = -n
		if n&1 == 1 {
			sign = true
		}
	}
	if n == 1 {
		if sign {
			return -gmY1(x)
		}
		return gmY1(x)
	}
	var b float64
	if x >= Two302 {

		var temp float64
		switch s, c := gmSincos(x); n & 3 {
		case 0:
			temp = s - c
		case 1:
			temp = -s - c
		case 2:
			temp = -s + c
		case 3:
			temp = s + c
		}
		b = float64((1/gmSqrtPi)*temp) / gmSqrt(x)
	} else {
		a := gmY0(x)
		b = gmY1(x)

		for i := 1; i < n && !gmIsInf(b, -1); i++ {
			a, b = b, float64((float64(i+i)/x)*b)-a
		}
	}
	if sign {
		return -b
	}
	return b
}

func gmLdexp(frac float64, exp int) float64 {
	if gmhaveArchLdexp {
		return gmarchLdexp(frac, exp)
	}
	return gmldexp(frac, exp)
}

func gmldexp(frac float64, exp int) float64 {

	switch {
	case frac == 0:
		return frac
	case gmIsInf(frac, 0) || gmIsNaN(frac):
		return frac
	}
	frac, e := gmnormalize(frac)
	exp += e
	x := gmFloat64bits(frac)
	exp += int(x>>gmshift)&gmmask - gmbias
	if exp < -1075 {
		return gmCopysign(0, frac)
	}
	if exp > 1023 {
		if frac < 0 {
			return gmInf(-1)
		}
		return gmInf(1)
	}
	var m float64 = 1
	if exp < -1022 {
		exp += 53
		m = 1.0 / (1 << 53)
	}
	x &^= gmmask << gmshift
	x |= uint64(exp+gmbias) << gmshift
	return float64(m * gmFloat64frombits(x))
}

var gm_lgamA = [...]float64{
	7.72156649015328655494e-02,
	3.22467033424113591611e-01,
	6.73523010531292681824e-02,
	2.05808084325167332806e-02,
	7.38555086081402883957e-03,
	2.89051383673415629091e-03,
	1.19270763183362067845e-03,
	5.10069792153511336608e-04,
	2.20862790713908385557e-04,
	1.08011567247583939954e-04,
	2.52144565451257326939e-05,
	4.48640949618915160150e-05,
}

var gm_lgamR = [...]float64{
	1.0,
	1.39200533467621045958e+00,
	7.21935547567138069525e-01,
	1.71933865632803078993e-01,
	1.86459191715652901344e-02,
	7.77942496381893596434e-04,
	7.32668430744625636189e-06,
}

var gm_lgamS = [...]float64{
	-7.72156649015328655494e-02,
	2.14982415960608852501e-01,
	3.25778796408930981787e-01,
	1.46350472652464452805e-01,
	2.66422703033638609560e-02,
	1.84028451407337715652e-03,
	3.19475326584100867617e-05,
}

var gm_lgamT = [...]float64{
	4.83836122723810047042e-01,
	-1.47587722994593911752e-01,
	6.46249402391333854778e-02,
	-3.27885410759859649565e-02,
	1.79706750811820387126e-02,
	-1.03142241298341437450e-02,
	6.10053870246291332635e-03,
	-3.68452016781138256760e-03,
	2.25964780900612472250e-03,
	-1.40346469989232843813e-03,
	8.81081882437654011382e-04,
	-5.38595305356740546715e-04,
	3.15632070903625950361e-04,
	-3.12754168375120860518e-04,
	3.35529192635519073543e-04,
}

var gm_lgamU = [...]float64{
	-7.72156649015328655494e-02,
	6.32827064025093366517e-01,
	1.45492250137234768737e+00,
	9.77717527963372745603e-01,
	2.28963728064692451092e-01,
	1.33810918536787660377e-02,
}

var gm_lgamV = [...]float64{
	1.0,
	2.45597793713041134822e+00,
	2.12848976379893395361e+00,
	7.69285150456672783825e-01,
	1.04222645593369134254e-01,
	3.21709242282423911810e-03,
}

var gm_lgamW = [...]float64{
	4.18938533204672725052e-01,
	8.33333333333329678849e-02,
	-2.77777777728775536470e-03,
	7.93650558643019558500e-04,
	-5.95187557450339963135e-04,
	8.36339918996282139126e-04,
	-1.63092934096575273989e-03,
}

func gmLgamma(x float64) (lgamma float64, sign int) {
	const (
		Ymin  = 1.461632144968362245
		Two52 = 1 << 52
		Two53 = 1 << 53
		Two58 = 1 << 58
		Tiny  = 1.0 / (1 << 70)
		Tc    = 1.46163214496836224576e+00
		Tf    = -1.21486290535849611461e-01

		Tt = -3.63867699703950536541e-18
	)

	sign = 1
	switch {
	case gmIsNaN(x):
		lgamma = x
		return
	case gmIsInf(x, 0):
		lgamma = x
		return
	case x == 0:
		lgamma = gmInf(1)
		return
	}

	neg := false
	if x < 0 {
		x = -x
		neg = true
	}

	if x < Tiny {
		if neg {
			sign = -1
		}
		lgamma = -gmLog(x)
		return
	}
	var nadj float64
	if neg {
		if x >= Two52 {
			lgamma = gmInf(1)
			return
		}
		t := gmsinPi(x)
		if t == 0 {
			lgamma = gmInf(1)
			return
		}
		nadj = gmLog(gmPi / gmAbs(float64(t*x)))
		if t < 0 {
			sign = -1
		}
	}

	switch {
	case x == 1 || x == 2:
		lgamma = 0
		return
	case x < 2:
		var y float64
		var i int
		if x <= 0.9 {
			lgamma = -gmLog(x)
			switch {
			case x >= (Ymin - 1 + 0.27):
				y = 1 - x
				i = 0
			case x >= (Ymin - 1 - 0.27):
				y = x - (Tc - 1)
				i = 1
			default:
				y = x
				i = 2
			}
		} else {
			lgamma = 0
			switch {
			case x >= (Ymin + 0.27):
				y = 2 - x
				i = 0
			case x >= (Ymin - 0.27):
				y = x - Tc
				i = 1
			default:
				y = x - 1
				i = 2
			}
		}
		switch i {
		case 0:
			z := float64(y * y)
			p1 := gm_lgamA[0] + float64(z*(gm_lgamA[2]+float64(z*(gm_lgamA[4]+float64(z*(gm_lgamA[6]+float64(z*(gm_lgamA[8]+float64(z*gm_lgamA[10])))))))))
			p2 := float64(z * (gm_lgamA[1] + float64(z*(+gm_lgamA[3]+float64(z*(gm_lgamA[5]+float64(z*(gm_lgamA[7]+float64(z*(gm_lgamA[9]+float64(z*gm_lgamA[11])))))))))))
			p := float64(y*p1) + p2
			lgamma += (p - float64(0.5*y))
		case 1:
			z := float64(y * y)
			w := float64(z * y)
			p1 := gm_lgamT[0] + float64(w*(gm_lgamT[3]+float64(w*(gm_lgamT[6]+float64(w*(gm_lgamT[9]+float64(w*gm_lgamT[12])))))))
			p2 := gm_lgamT[1] + float64(w*(gm_lgamT[4]+float64(w*(gm_lgamT[7]+float64(w*(gm_lgamT[10]+float64(w*gm_lgamT[13])))))))
			p3 := gm_lgamT[2] + float64(w*(gm_lgamT[5]+float64(w*(gm_lgamT[8]+float64(w*(gm_lgamT[11]+float64(w*gm_lgamT[14])))))))
			p := float64(z*p1) - (Tt - float64(w*(p2+float64(y*p3))))
			lgamma += (Tf + p)
		case 2:
			p1 := float64(y * (gm_lgamU[0] + float64(y*(gm_lgamU[1]+float64(y*(gm_lgamU[2]+float64(y*(gm_lgamU[3]+float64(y*(gm_lgamU[4]+float64(y*gm_lgamU[5])))))))))))
			p2 := 1 + float64(y*(gm_lgamV[1]+float64(y*(gm_lgamV[2]+float64(y*(gm_lgamV[3]+float64(y*(gm_lgamV[4]+float64(y*gm_lgamV[5])))))))))
			lgamma += (float64(-0.5*y) + p1/p2)
		}
	case x < 8:
		i := int(x)
		y := x - float64(i)
		p := float64(y * (gm_lgamS[0] + float64(y*(gm_lgamS[1]+float64(y*(gm_lgamS[2]+float64(y*(gm_lgamS[3]+float64(y*(gm_lgamS[4]+float64(y*(gm_lgamS[5]+float64(y*gm_lgamS[6])))))))))))))
		q := 1 + float64(y*(gm_lgamR[1]+float64(y*(gm_lgamR[2]+float64(y*(gm_lgamR[3]+float64(y*(gm_lgamR[4]+float64(y*(gm_lgamR[5]+float64(y*gm_lgamR[6])))))))))))
		lgamma = float64(0.5*y) + p/q
		z := 1.0
		switch i {
		case 7:
			z *= (y + 6)
			fallthrough
		case 6:
			z *= (y + 5)
			fallthrough
		case 5:
			z *= (y + 4)
			fallthrough
		case 4:
			z *= (y + 3)
			fallthrough
		case 3:
			z *= (y + 2)
			lgamma += gmLog(z)
		}
	case x < Two58:
		t := gmLog(x)
		z := 1 / x
		y := float64(z * z)
		w := gm_lgamW[0] + float64(z*(gm_lgamW[1]+float64(y*(gm_lgamW[2]+float64(y*(gm_lgamW[3]+float64(y*(gm_lgamW[4]+float64(y*(gm_lgamW[5]+float64(y*gm_lgamW[6])))))))))))
		lgamma = float64((x-0.5)*(t-1)) + w
	default:
		lgamma = float64(x * (gmLog(x) - 1))
	}
	if neg {
		lgamma = nadj - lgamma
	}
	return
}

func gmsinPi(x float64) float64 {
	const (
		Two52 = 1 << 52
		Two53 = 1 << 53
	)
	if x < 0.25 {
		return -gmSin(float64(gmPi * x))
	}

	z := gmFloor(x)
	var n int
	if z != x {
		x = gmMod(x, 2)
		n = int(float64(x * 4))
	} else {
		if x >= Two53 {
			x = 0
			n = 0
		} else {
			if x < Two52 {
				z = x + Two52
			}
			n = int(1 & gmFloat64bits(z))
			x = float64(n)
			n <<= 2
		}
	}
	switch n {
	case 0:
		x = gmSin(float64(gmPi * x))
	case 1, 2:
		x = gmCos(float64(gmPi * (0.5 - x)))
	case 3, 4:
		x = gmSin(float64(gmPi * (1 - x)))
	case 5, 6:
		x = -gmCos(float64(gmPi * (x - 1.5)))
	default:
		x = gmSin(float64(gmPi * (x - 2)))
	}
	return -x
}

func gmLog(x float64) float64 {
	if gmhaveArchLog {
		return gmarchLog(x)
	}
	return gmlog(x)
}

func gmlog(x float64) float64 {
	const (
		Ln2Hi = 6.93147180369123816490e-01
		Ln2Lo = 1.90821492927058770002e-10
		L1    = 6.666666666666735130e-01
		L2    = 3.999999999940941908e-01
		L3    = 2.857142874366239149e-01
		L4    = 2.222219843214978396e-01
		L5    = 1.818357216161805012e-01
		L6    = 1.531383769920937332e-01
		L7    = 1.479819860511658591e-01
	)

	switch {
	case gmIsNaN(x) || gmIsInf(x, 1):
		return x
	case x < 0:
		return gmNaN()
	case x == 0:
		return gmInf(-1)
	}

	f1, ki := gmFrexp(x)
	if f1 < gmSqrt2/2 {
		f1 *= 2
		ki--
	}
	f := f1 - 1
	k := float64(ki)

	s := f / (2 + f)
	s2 := float64(s * s)
	s4 := float64(s2 * s2)
	t1 := float64(s2 * (L1 + float64(s4*(L3+float64(s4*(L5+float64(s4*L7)))))))
	t2 := float64(s4 * (L2 + float64(s4*(L4+float64(s4*L6)))))
	R := t1 + t2
	hfsq := float64(float64(0.5*f) * f)
	return float64(k*Ln2Hi) - ((hfsq - (float64(s*(hfsq+R)) + float64(k*Ln2Lo))) - f)
}

func gmLog10(x float64) float64 {
	if gmhaveArchLog10 {
		return gmarchLog10(x)
	}
	return gmlog10(x)
}

func gmlog10(x float64) float64 {
	return float64(gmLog(x) * (1 / gmLn10))
}

func gmLog2(x float64) float64 {
	if gmhaveArchLog2 {
		return gmarchLog2(x)
	}
	return gmlog2(x)
}

func gmlog2(x float64) float64 {
	frac, exp := gmFrexp(x)

	if frac == 0.5 {
		return float64(exp - 1)
	}
	return float64(gmLog(frac)*(1/gmLn2)) + float64(exp)
}

func gmLog1p(x float64) float64 {
	if gmhaveArchLog1p {
		return gmarchLog1p(x)
	}
	return gmlog1p(x)
}

func gmlog1p(x float64) float64 {
	const (
		Sqrt2M1     = 4.142135623730950488017e-01
		Sqrt2HalfM1 = -2.928932188134524755992e-01
		Small       = 1.0 / (1 << 29)
		Tiny        = 1.0 / (1 << 54)
		Two53       = 1 << 53
		Ln2Hi       = 6.93147180369123816490e-01
		Ln2Lo       = 1.90821492927058770002e-10
		Lp1         = 6.666666666666735130e-01
		Lp2         = 3.999999999940941908e-01
		Lp3         = 2.857142874366239149e-01
		Lp4         = 2.222219843214978396e-01
		Lp5         = 1.818357216161805012e-01
		Lp6         = 1.531383769920937332e-01
		Lp7         = 1.479819860511658591e-01
	)

	switch {
	case x < -1 || gmIsNaN(x):
		return gmNaN()
	case x == -1:
		return gmInf(-1)
	case gmIsInf(x, 1):
		return gmInf(1)
	}

	absx := gmAbs(x)

	var f float64
	var iu uint64
	k := 1
	if absx < Sqrt2M1 {
		if absx < Small {
			if absx < Tiny {
				return x
			}
			return x - float64(float64(x*x)*0.5)
		}
		if x > Sqrt2HalfM1 {

			k = 0
			f = x
			iu = 1
		}
	}
	var c float64
	if k != 0 {
		var u float64
		if absx < Two53 {
			u = 1.0 + x
			iu = gmFloat64bits(u)
			k = int((iu >> 52) - 1023)

			if k > 0 {
				c = 1.0 - (u - x)
			} else {
				c = x - (u - 1.0)
			}
			c /= u
		} else {
			u = x
			iu = gmFloat64bits(u)
			k = int((iu >> 52) - 1023)
			c = 0
		}
		iu &= 0x000fffffffffffff
		if iu < 0x0006a09e667f3bcd {
			u = gmFloat64frombits(iu | 0x3ff0000000000000)
		} else {
			k++
			u = gmFloat64frombits(iu | 0x3fe0000000000000)
			iu = (0x0010000000000000 - iu) >> 2
		}
		f = u - 1.0
	}
	hfsq := float64(float64(0.5*f) * f)
	var s, R, z float64
	if iu == 0 {
		if f == 0 {
			if k == 0 {
				return 0
			}
			c += float64(float64(k) * Ln2Lo)
			return float64(float64(k)*Ln2Hi) + c
		}
		R = float64(hfsq * (1.0 - float64(0.66666666666666666*f)))
		if k == 0 {
			return f - R
		}
		return float64(float64(k)*Ln2Hi) - ((R - (float64(float64(k)*Ln2Lo) + c)) - f)
	}
	s = f / (2.0 + f)
	z = float64(s * s)
	R = float64(z * (Lp1 + float64(z*(Lp2+float64(z*(Lp3+float64(z*(Lp4+float64(z*(Lp5+float64(z*(Lp6+float64(z*Lp7)))))))))))))
	if k == 0 {
		return f - (hfsq - float64(s*(hfsq+R)))
	}
	return float64(float64(k)*Ln2Hi) - ((hfsq - (float64(s*(hfsq+R)) + (float64(float64(k)*Ln2Lo) + c))) - f)
}

const gmhaveArchLog = false

func gmarchLog(x float64) float64 {
	panic("not implemented")
}

func gmLogb(x float64) float64 {

	switch {
	case x == 0:
		return gmInf(-1)
	case gmIsInf(x, 0):
		return gmInf(1)
	case gmIsNaN(x):
		return x
	}
	return float64(gmilogb(x))
}

func gmIlogb(x float64) int {

	switch {
	case x == 0:
		return gmMinInt32
	case gmIsNaN(x):
		return gmMaxInt32
	case gmIsInf(x, 0):
		return gmMaxInt32
	}
	return gmilogb(x)
}

func gmilogb(x float64) int {
	x, exp := gmnormalize(x)
	return int((gmFloat64bits(x)>>gmshift)&gmmask) - gmbias + exp
}

func gmMod(x, y float64) float64 {
	if gmhaveArchMod {
		return gmarchMod(x, y)
	}
	return gmmod(x, y)
}

func gmmod(x, y float64) float64 {
	if y == 0 || gmIsInf(x, 0) || gmIsNaN(x) || gmIsNaN(y) {
		return gmNaN()
	}
	y = gmAbs(y)

	yfr, yexp := gmFrexp(y)
	r := x
	if x < 0 {
		r = -x
	}

	for r >= y {
		rfr, rexp := gmFrexp(r)
		if rfr < yfr {
			rexp = rexp - 1
		}
		r = r - gmLdexp(y, rexp-yexp)
	}
	if x < 0 {
		r = -r
	}
	return r
}

func gmModf(f float64) (integer float64, fractional float64) {
	integer = gmTrunc(f)
	fractional = gmCopysign(f-integer, f)
	return
}

func gmNextafter32(x, y float32) (r float32) {
	switch {
	case gmIsNaN(float64(x)) || gmIsNaN(float64(y)):
		r = float32(gmNaN())
	case x == y:
		r = x
	case x == 0:
		r = float32(gmCopysign(float64(gmFloat32frombits(1)), float64(y)))
	case (y > x) == (x > 0):
		r = gmFloat32frombits(gmFloat32bits(x) + 1)
	default:
		r = gmFloat32frombits(gmFloat32bits(x) - 1)
	}
	return
}

func gmNextafter(x, y float64) (r float64) {
	switch {
	case gmIsNaN(x) || gmIsNaN(y):
		r = gmNaN()
	case x == y:
		r = x
	case x == 0:
		r = gmCopysign(gmFloat64frombits(1), y)
	case (y > x) == (x > 0):
		r = gmFloat64frombits(gmFloat64bits(x) + 1)
	default:
		r = gmFloat64frombits(gmFloat64bits(x) - 1)
	}
	return
}

func gmisOddInt(x float64) bool {
	if gmAbs(x) >= (1 << 53) {

		return false
	}

	xi, xf := gmModf(x)
	return xf == 0 && int64(xi)&1 == 1
}

func gmPow(x, y float64) float64 {
	if gmhaveArchPow {
		return gmarchPow(x, y)
	}
	return gmpow(x, y)
}

func gmpow(x, y float64) float64 {
	switch {
	case y == 0 || x == 1:
		return 1
	case y == 1:
		return x
	case gmIsNaN(x) || gmIsNaN(y):
		return gmNaN()
	case x == 0:
		switch {
		case y < 0:
			if gmSignbit(x) && gmisOddInt(y) {
				return gmInf(-1)
			}
			return gmInf(1)
		case y > 0:
			if gmSignbit(x) && gmisOddInt(y) {
				return x
			}
			return 0
		}
	case gmIsInf(y, 0):
		switch {
		case x == -1:
			return 1
		case (gmAbs(x) < 1) == gmIsInf(y, 1):
			return 0
		default:
			return gmInf(1)
		}
	case gmIsInf(x, 0):
		if gmIsInf(x, -1) {
			return gmPow(1/x, -y)
		}
		switch {
		case y < 0:
			return 0
		case y > 0:
			return gmInf(1)
		}
	case y == 0.5:
		return gmSqrt(x)
	case y == -0.5:
		return 1 / gmSqrt(x)
	}

	yi, yf := gmModf(gmAbs(y))
	if yf != 0 && x < 0 {
		return gmNaN()
	}
	if yi >= 1<<63 {

		switch {
		case x == -1:
			return 1
		case (gmAbs(x) < 1) == (y > 0):
			return 0
		default:
			return gmInf(1)
		}
	}

	a1 := 1.0
	ae := 0

	if yf != 0 {
		if yf > 0.5 {
			yf--
			yi++
		}
		a1 = gmExp(float64(yf * gmLog(x)))
	}

	x1, xe := gmFrexp(x)
	for i := int64(yi); i != 0; i >>= 1 {
		if xe < -1<<12 || 1<<12 < xe {

			ae += xe
			break
		}
		if i&1 == 1 {
			a1 *= x1
			ae += xe
		}
		x1 *= x1
		xe <<= 1
		if x1 < .5 {
			x1 += x1
			xe--
		}
	}

	if y < 0 {
		a1 = 1 / a1
		ae = -ae
	}
	return gmLdexp(a1, ae)
}

var gmpow10tab = [...]float64{
	1e00, 1e01, 1e02, 1e03, 1e04, 1e05, 1e06, 1e07, 1e08, 1e09,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
	1e20, 1e21, 1e22, 1e23, 1e24, 1e25, 1e26, 1e27, 1e28, 1e29,
	1e30, 1e31,
}

var gmpow10postab32 = [...]float64{
	1e00, 1e32, 1e64, 1e96, 1e128, 1e160, 1e192, 1e224, 1e256, 1e288,
}

var gmpow10negtab32 = [...]float64{
	1e-00, 1e-32, 1e-64, 1e-96, 1e-128, 1e-160, 1e-192, 1e-224, 1e-256, 1e-288, 1e-320,
}

func gmPow10(n int) float64 {
	if 0 <= n && n <= 308 {
		return float64(gmpow10postab32[uint(n)/32] * gmpow10tab[uint(n)%32])
	}

	if -323 <= n && n <= 0 {
		return gmpow10negtab32[uint(-n)/32] / gmpow10tab[uint(-n)%32]
	}

	if n > 0 {
		return gmInf(1)
	}

	return 0
}

func gmRemainder(x, y float64) float64 {
	if gmhaveArchRemainder {
		return gmarchRemainder(x, y)
	}
	return gmremainder(x, y)
}

func gmremainder(x, y float64) float64 {
	const (
		Tiny    = 4.45014771701440276618e-308
		HalfMax = gmMaxFloat64 / 2
	)

	switch {
	case gmIsNaN(x) || gmIsNaN(y) || gmIsInf(x, 0) || y == 0:
		return gmNaN()
	case gmIsInf(y, 0):
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if y < 0 {
		y = -y
	}
	if x == y {
		if sign {
			zero := 0.0
			return -zero
		}
		return 0
	}
	if y <= HalfMax {
		x = gmMod(x, y+y)
	}
	if y < Tiny {
		if x+x > y {
			x -= y
			if x+x >= y {
				x -= y
			}
		}
	} else {
		yHalf := float64(0.5 * y)
		if x > yHalf {
			x -= y
			if x >= yHalf {
				x -= y
			}
		}
	}
	if sign {
		x = -x
	}
	return x
}

func gmSignbit(x float64) bool {
	return int64(gmFloat64bits(x)) < 0
}

var gm_sin = [...]float64{
	1.58962301576546568060e-10,
	-2.50507477628578072866e-8,
	2.75573136213857245213e-6,
	-1.98412698295895385996e-4,
	8.33333333332211858878e-3,
	-1.66666666666666307295e-1,
}

var gm_cos = [...]float64{
	-1.13585365213876817300e-11,
	2.08757008419747316778e-9,
	-2.75573141792967388112e-7,
	2.48015872888517045348e-5,
	-1.38888888888730564116e-3,
	4.16666666666665929218e-2,
}

func gmCos(x float64) float64 {
	if gmhaveArchCos {
		return gmarchCos(x)
	}
	return gmcos(x)
}

func gmcos(x float64) float64 {
	const (
		PI4A = 7.85398125648498535156e-1
		PI4B = 3.77489470793079817668e-8
		PI4C = 2.69515142907905952645e-15
	)

	switch {
	case gmIsNaN(x) || gmIsInf(x, 0):
		return gmNaN()
	}

	sign := false
	x = gmAbs(x)

	var j uint64
	var y, z float64
	if x >= gmreduceThreshold {
		j, z = gmtrigReduce(x)
	} else {
		j = uint64(float64(x * (4 / gmPi)))
		y = float64(j)

		if j&1 == 1 {
			j++
			y++
		}
		j &= 7
		z = ((x - float64(y*PI4A)) - float64(y*PI4B)) - float64(y*PI4C)
	}

	if j > 3 {
		j -= 4
		sign = !sign
	}
	if j > 1 {
		sign = !sign
	}

	zz := float64(z * z)
	if j == 1 || j == 2 {
		y = z + float64(float64(z*zz)*(float64((float64((float64((float64(((float64(gm_sin[0]*zz))+gm_sin[1])*zz)+gm_sin[2])*zz)+gm_sin[3])*zz)+gm_sin[4])*zz)+gm_sin[5]))
	} else {
		y = 1.0 - float64(0.5*zz) + float64(float64(zz*zz)*(float64((float64((float64((float64(((float64(gm_cos[0]*zz))+gm_cos[1])*zz)+gm_cos[2])*zz)+gm_cos[3])*zz)+gm_cos[4])*zz)+gm_cos[5]))
	}
	if sign {
		y = -y
	}
	return y
}

func gmSin(x float64) float64 {
	if gmhaveArchSin {
		return gmarchSin(x)
	}
	return gmsin(x)
}

func gmsin(x float64) float64 {
	const (
		PI4A = 7.85398125648498535156e-1
		PI4B = 3.77489470793079817668e-8
		PI4C = 2.69515142907905952645e-15
	)

	switch {
	case x == 0 || gmIsNaN(x):
		return x
	case gmIsInf(x, 0):
		return gmNaN()
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}

	var j uint64
	var y, z float64
	if x >= gmreduceThreshold {
		j, z = gmtrigReduce(x)
	} else {
		j = uint64(float64(x * (4 / gmPi)))
		y = float64(j)

		if j&1 == 1 {
			j++
			y++
		}
		j &= 7
		z = ((x - float64(y*PI4A)) - float64(y*PI4B)) - float64(y*PI4C)
	}

	if j > 3 {
		sign = !sign
		j -= 4
	}
	zz := float64(z * z)
	if j == 1 || j == 2 {
		y = 1.0 - float64(0.5*zz) + float64(float64(zz*zz)*(float64((float64((float64((float64(((float64(gm_cos[0]*zz))+gm_cos[1])*zz)+gm_cos[2])*zz)+gm_cos[3])*zz)+gm_cos[4])*zz)+gm_cos[5]))
	} else {
		y = z + float64(float64(z*zz)*(float64((float64((float64((float64(((float64(gm_sin[0]*zz))+gm_sin[1])*zz)+gm_sin[2])*zz)+gm_sin[3])*zz)+gm_sin[4])*zz)+gm_sin[5]))
	}
	if sign {
		y = -y
	}
	return y
}

func gmSincos(x float64) (sin, cos float64) {
	const (
		PI4A = 7.85398125648498535156e-1
		PI4B = 3.77489470793079817668e-8
		PI4C = 2.69515142907905952645e-15
	)

	switch {
	case x == 0:
		return x, 1
	case gmIsNaN(x) || gmIsInf(x, 0):
		return gmNaN(), gmNaN()
	}

	sinSign, cosSign := false, false
	if x < 0 {
		x = -x
		sinSign = true
	}

	var j uint64
	var y, z float64
	if x >= gmreduceThreshold {
		j, z = gmtrigReduce(x)
	} else {
		j = uint64(float64(x * (4 / gmPi)))
		y = float64(j)

		if j&1 == 1 {
			j++
			y++
		}
		j &= 7
		z = ((x - float64(y*PI4A)) - float64(y*PI4B)) - float64(y*PI4C)
	}
	if j > 3 {
		j -= 4
		sinSign, cosSign = !sinSign, !cosSign
	}
	if j > 1 {
		cosSign = !cosSign
	}

	zz := float64(z * z)
	cos = 1.0 - float64(0.5*zz) + float64(float64(zz*zz)*(float64((float64((float64((float64(((float64(gm_cos[0]*zz))+gm_cos[1])*zz)+gm_cos[2])*zz)+gm_cos[3])*zz)+gm_cos[4])*zz)+gm_cos[5]))
	sin = z + float64(float64(z*zz)*(float64((float64((float64((float64(((float64(gm_sin[0]*zz))+gm_sin[1])*zz)+gm_sin[2])*zz)+gm_sin[3])*zz)+gm_sin[4])*zz)+gm_sin[5]))
	if j == 1 || j == 2 {
		sin, cos = cos, sin
	}
	if cosSign {
		cos = -cos
	}
	if sinSign {
		sin = -sin
	}
	return
}

func gmSinh(x float64) float64 {
	if gmhaveArchSinh {
		return gmarchSinh(x)
	}
	return gmsinh(x)
}

func gmsinh(x float64) float64 {

	const (
		P0 = -0.6307673640497716991184787251e+6
		P1 = -0.8991272022039509355398013511e+5
		P2 = -0.2894211355989563807284660366e+4
		P3 = -0.2630563213397497062819489e+2
		Q0 = -0.6307673640497716991212077277e+6
		Q1 = 0.1521517378790019070696485176e+5
		Q2 = -0.173678953558233699533450911e+3
	)

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}

	var temp float64
	switch {
	case x > 21:
		temp = float64(gmExp(x) * 0.5)

	case x > 0.5:
		ex := gmExp(x)
		temp = float64((ex - 1/ex) * 0.5)

	default:
		sq := float64(x * x)
		temp = float64((float64((float64((float64(P3*sq)+P2)*sq)+P1)*sq) + P0) * x)
		temp = temp / (float64((float64((sq+Q2)*sq)+Q1)*sq) + Q0)
	}

	if sign {
		temp = -temp
	}
	return temp
}

func gmCosh(x float64) float64 {
	if gmhaveArchCosh {
		return gmarchCosh(x)
	}
	return gmcosh(x)
}

func gmcosh(x float64) float64 {
	x = gmAbs(x)
	if x > 21 {
		return float64(gmExp(x) * 0.5)
	}
	ex := gmExp(x)
	return float64((ex + 1/ex) * 0.5)
}

func gmSqrt(x float64) float64 {
	return gmsqrt(x)
}

func gmsqrt(x float64) float64 {

	switch {
	case x == 0 || gmIsNaN(x) || gmIsInf(x, 1):
		return x
	case x < 0:
		return gmNaN()
	}
	ix := gmFloat64bits(x)

	exp := int((ix >> gmshift) & gmmask)
	if exp == 0 {
		for ix&(1<<gmshift) == 0 {
			ix <<= 1
			exp--
		}
		exp++
	}
	exp -= gmbias
	ix &^= gmmask << gmshift
	ix |= 1 << gmshift
	if exp&1 == 1 {
		ix <<= 1
	}
	exp >>= 1

	ix <<= 1
	var q, s uint64
	r := uint64(1 << (gmshift + 1))
	for r != 0 {
		t := s + r
		if t <= ix {
			s = t + r
			ix -= t
			q += r
		}
		ix <<= 1
		r >>= 1
	}

	if ix != 0 {
		q += q & 1
	}
	ix = q>>1 + uint64(exp-1+gmbias)<<gmshift
	return gmFloat64frombits(ix)
}

const gmhaveArchAcos = false

func gmarchAcos(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAcosh = false

func gmarchAcosh(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAsin = false

func gmarchAsin(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAsinh = false

func gmarchAsinh(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAtan = false

func gmarchAtan(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAtan2 = false

func gmarchAtan2(y, x float64) float64 {
	panic("not implemented")
}

const gmhaveArchAtanh = false

func gmarchAtanh(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchCbrt = false

func gmarchCbrt(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchCos = false

func gmarchCos(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchCosh = false

func gmarchCosh(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchErf = false

func gmarchErf(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchErfc = false

func gmarchErfc(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchExpm1 = false

func gmarchExpm1(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchFrexp = false

func gmarchFrexp(x float64) (float64, int) {
	panic("not implemented")
}

const gmhaveArchLdexp = false

func gmarchLdexp(frac float64, exp int) float64 {
	panic("not implemented")
}

const gmhaveArchLog10 = false

func gmarchLog10(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchLog2 = false

func gmarchLog2(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchLog1p = false

func gmarchLog1p(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchMod = false

func gmarchMod(x, y float64) float64 {
	panic("not implemented")
}

const gmhaveArchPow = false

func gmarchPow(x, y float64) float64 {
	panic("not implemented")
}

const gmhaveArchRemainder = false

func gmarchRemainder(x, y float64) float64 {
	panic("not implemented")
}

const gmhaveArchSin = false

func gmarchSin(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchSinh = false

func gmarchSinh(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchTan = false

func gmarchTan(x float64) float64 {
	panic("not implemented")
}

const gmhaveArchTanh = false

func gmarchTanh(x float64) float64 {
	panic("not implemented")
}

var gm_tanP = [...]float64{
	-1.30936939181383777646e4,
	1.15351664838587416140e6,
	-1.79565251976484877988e7,
}

var gm_tanQ = [...]float64{
	1.00000000000000000000e0,
	1.36812963470692954678e4,
	-1.32089234440210967447e6,
	2.50083801823357915839e7,
	-5.38695755929454629881e7,
}

func gmTan(x float64) float64 {
	if gmhaveArchTan {
		return gmarchTan(x)
	}
	return gmtan(x)
}

func gmtan(x float64) float64 {
	const (
		PI4A = 7.85398125648498535156e-1
		PI4B = 3.77489470793079817668e-8
		PI4C = 2.69515142907905952645e-15
	)

	switch {
	case x == 0 || gmIsNaN(x):
		return x
	case gmIsInf(x, 0):
		return gmNaN()
	}

	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var j uint64
	var y, z float64
	if x >= gmreduceThreshold {
		j, z = gmtrigReduce(x)
	} else {
		j = uint64(float64(x * (4 / gmPi)))
		y = float64(j)

		if j&1 == 1 {
			j++
			y++
		}

		z = ((x - float64(y*PI4A)) - float64(y*PI4B)) - float64(y*PI4C)
	}
	zz := float64(z * z)

	if zz > 1e-14 {
		y = z + float64(z*(float64(zz*(float64(((float64(gm_tanP[0]*zz))+gm_tanP[1])*zz)+gm_tanP[2]))/(float64((float64((float64((zz+gm_tanQ[1])*zz)+gm_tanQ[2])*zz)+gm_tanQ[3])*zz)+gm_tanQ[4])))
	} else {
		y = z
	}
	if j&2 == 2 {
		y = -1 / y
	}
	if sign {
		y = -y
	}
	return y
}

var gmtanhP = [...]float64{
	-9.64399179425052238628e-1,
	-9.92877231001918586564e1,
	-1.61468768441708447952e3,
}

var gmtanhQ = [...]float64{
	1.12811678491632931402e2,
	2.23548839060100448583e3,
	4.84406305325125486048e3,
}

func gmTanh(x float64) float64 {
	if gmhaveArchTanh {
		return gmarchTanh(x)
	}
	return gmtanh(x)
}

func gmtanh(x float64) float64 {
	const MAXLOG = 8.8029691931113054295988e+01
	z := gmAbs(x)
	switch {
	case z > 0.5*MAXLOG:
		if x < 0 {
			return -1
		}
		return 1
	case z >= 0.625:
		s := gmExp(float64(2 * z))
		z = 1 - 2/(s+1)
		if x < 0 {
			z = -z
		}
	default:
		if x == 0 {
			return x
		}
		s := float64(x * x)
		z = x + float64(float64(x*s)*(float64((float64(gmtanhP[0]*s)+gmtanhP[1])*s)+gmtanhP[2]))/(float64((float64((s+gmtanhQ[0])*s)+gmtanhQ[1])*s)+gmtanhQ[2])
	}
	return z
}

const gmreduceThreshold = 1 << 29

func gmtrigReduce(x float64) (j uint64, z float64) {
	const PI4 = gmPi / 4
	if x < PI4 {
		return 0, x
	}

	ix := gmFloat64bits(x)
	exp := int(ix>>gmshift&gmmask) - gmbias - gmshift
	ix &^= gmmask << gmshift
	ix |= 1 << gmshift

	digit, bitshift := uint(exp+61)/64, uint(exp+61)%64
	z0 := (gmmPi4[digit] << bitshift) | (gmmPi4[digit+1] >> (64 - bitshift))
	z1 := (gmmPi4[digit+1] << bitshift) | (gmmPi4[digit+2] >> (64 - bitshift))
	z2 := (gmmPi4[digit+2] << bitshift) | (gmmPi4[digit+3] >> (64 - bitshift))

	z2hi, _ := bits.Mul64(z2, ix)
	z1hi, z1lo := bits.Mul64(z1, ix)
	z0lo := z0 * ix
	lo, c := bits.Add64(z1lo, z2hi, 0)
	hi, _ := bits.Add64(z0lo, z1hi, c)

	j = hi >> 61

	hi = hi<<3 | lo>>61
	lz := uint(bits.LeadingZeros64(hi))
	e := uint64(gmbias - (lz + 1))

	hi = (hi << (lz + 1)) | (lo >> (64 - (lz + 1)))
	hi >>= 64 - gmshift

	hi |= e << gmshift
	z = gmFloat64frombits(hi)

	if j&1 == 1 {
		j++
		j &= 7
		z--
	}

	return j, float64(z * PI4)
}

var gmmPi4 = [...]uint64{
	0x0000000000000001,
	0x45f306dc9c882a53,
	0xf84eafa3ea69bb81,
	0xb6c52b3278872083,
	0xfca2c757bd778ac3,
	0x6e48dc74849ba5c0,
	0x0c925dd413a32439,
	0xfc3bd63962534e7d,
	0xd1046bea5d768909,
	0xd338e04d68befc82,
	0x7323ac7306a673e9,
	0x3908bf177bf25076,
	0x3ff12fffbc0b301f,
	0xde5e2316b414da3e,
	0xda6cfd9e4f96136e,
	0x9e8c7ecd3cbfd45a,
	0xea4f758fd7cbe2f6,
	0x7a0e73ef14a525d4,
	0xd7f6bf623f1aba10,
	0xac06608df8f6d757,
}

func gmFloat32bits(f float32) uint32 { return math.Float32bits(f) }

func gmFloat32frombits(b uint32) float32 { return math.Float32frombits(b) }

func gmFloat64bits(f float64) uint64 { return math.Float64bits(f) }

func gmFloat64frombits(b uint64) float64 { return math.Float64frombits(b) }
