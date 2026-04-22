package slot

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/slotopol/server/util"
	"gopkg.in/yaml.v3"
)

// Counter is an interface for slot game statistics collectors.
// It is implemented by StatGeneric and StatCascade.
type Counter interface {
	Count() float64
	SumPays() float64
	NSQ(cost float64) (N float64, S float64, Q float64)
	FSQ() (q float64, sq float64)
	FGQ() float64
	ΣPL(scat Sym, L []int) float64
}

// EvD calculates expected RTP (µ) and variance (D) from counter statistics.
func EvD(s Counter, cost float64) (µ float64, D float64) {
	var N, S, Q = s.NSQ(cost)
	µ = S / N
	D = Q/N - µ*µ
	return
}

func Print_vi(w io.Writer, sp *ScanPar, D float64) {
	if !sp.IsVI() {
		return
	}
	var sigma = math.Sqrt(D)
	var vi = GetZ(sp.Conf) * sigma
	fmt.Fprintf(w, "sigma = %.6g, VI[%.4g%%] = %.6g (%s)\n", sigma, sp.Conf*100, vi, VIname5[VIclass5(sigma)])
}

func Print_ci(w io.Writer, sp *ScanPar, rtp, D float64) {
	if !sp.IsCI() {
		return
	}
	if rtp > RTPconv {
		return
	}
	var sigma = math.Sqrt(D)
	var ci = CI(sp.Conf, rtp, sigma)
	var BRci = BankrollPlayer(sp.Conf, rtp, sigma, ci)
	fmt.Fprintf(w, "CI[%.4g%%] = %d, bankroll[CI] = %.6g\n", sp.Conf*100, int(ci+0.5), BRci)
}

func Print_spread(w io.Writer, sp *ScanPar, rtp, D float64) {
	if !sp.IsSpread() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "RTP spread for spins number with confidence %.4g%%:\n", sp.Conf*100)
	var N = []int{1e3, 1e4, 1e5, 1e6, 1e7}
	var sigma = math.Sqrt(D)
	var vi = GetZ(sp.Conf) * sigma
	var ci = CI(sp.Conf, rtp, sigma)
	if ci < 1e7 {
		N = append(N, int(ci+0.5))
		sort.Ints(N)
	}
	for _, n := range N {
		var Δ = vi / math.Sqrt(float64(n))
		fmt.Fprintf(w, "%8d: %.2f%% ... %.2f%%\n", n, (rtp-Δ)*100, (rtp+Δ)*100)
	}
}

func p5f(p float64) string {
	if p != 0 {
		return fmt.Sprintf("%5.2f", p)
	} else {
		return "    0"
	}
}

func Print_symbols_generic(w io.Writer, sp *ScanPar, s *StatGeneric, rtp float64) {
	if !sp.IsSym() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "symbols contribution to payouts:\n")
	fmt.Fprintf(w, "sym rate%%  rtp%% |")
	for x := range s.S[0] {
		fmt.Fprintf(w, " %5d", x+1)
	}
	fmt.Fprintf(w, "\n")
	var sum = s.SumPays()
	for sym, pays := range s.S {
		var cs = s.SymPays(Sym(sym+1)) / sum
		fmt.Fprintf(w, "%2d: %s %s |", sym+1, p5f(cs*100), p5f(rtp*cs*100))
		for x := range pays {
			var cx = pays[x].Load() / sum
			fmt.Fprintf(w, " %s", p5f(cx*100))
		}
		fmt.Fprintf(w, "\n")
	}
}

func Print_symbols_cascade(w io.Writer, sp *ScanPar, s *StatCascade, rtp float64) {
	if !sp.IsSym() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "symbols contribution to payouts:\n")
	fmt.Fprintf(w, "sym rate%%  rtp%% |")
	for x := range s.Casc[0].S[0] {
		fmt.Fprintf(w, " %5d", x+1)
	}
	fmt.Fprintf(w, "\n")
	var sum = s.SumPays()
	for sym, pays := range s.Casc[0].S {
		var cs = s.SymPays(Sym(sym+1)) / sum
		fmt.Fprintf(w, "%2d: %s %s |", sym+1, p5f(cs*100), p5f(rtp*cs*100))
		for x := range pays {
			var cx float64
			for cfn := range s.Casc {
				cx += s.Casc[cfn].S[sym][x].Load()
			}
			cx /= sum
			fmt.Fprintf(w, " %s", p5f(cx*100))
		}
		fmt.Fprintf(w, "\n")
	}
}

func Print_contribution_falls(w io.Writer, sp *ScanPar, s *StatCascade, rtp float64) {
	if !sp.IsSym() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "cascades contribution to payouts:\n")
	fmt.Fprintf(w, "cfn rate%%  rtp%%\n")
	var sum = s.SumPays()
	for cfn := range s.Casc {
		var c = s.Casc[cfn].SumPays() / sum
		fmt.Fprintf(w, "%2d: %s %s\n", cfn+1, p5f(c*100), p5f(rtp*c*100))
		if c == 0 {
			break
		}
	}
}

func Print_raw(w io.Writer, sp *ScanPar, s any) {
	if !sp.IsRaw() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "simulator raw data:\n")
	var b, err = yaml.Marshal(s)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		return
	}
	fmt.Fprintf(w, util.B2S(b))
}

func Print_cascmetrics(w io.Writer, sp *ScanPar, s *StatCascade) {
	if !sp.IsCasc() {
		return
	}
	var N = s.Count()
	var N2 = float64(s.Casc[1].N.Load())
	var N3 = float64(s.Casc[2].N.Load())
	var N4 = float64(s.Casc[3].N.Load())
	var N5 = float64(s.Casc[4].N.Load())
	fmt.Fprintln(w)
	fmt.Fprintf(w, "cascade metrics:\n")
	fmt.Fprintf(w, "N[2] = %.10g, Ec2 = Kf2 = 1/%.5g\n", N2, N/N2)
	fmt.Fprintf(w, "N[3] = %.10g, Ec3 = 1/%.5g, Kf3 = 1/%.5g\n", N3, N/N3, N2/N3)
	fmt.Fprintf(w, "N[4] = %.10g, Ec4 = 1/%.5g, Kf4 = 1/%.5g\n", N4, N/N4, N3/N4)
	fmt.Fprintf(w, "N[5] = %.10g, Ec5 = 1/%.5g, Kf5 = 1/%.5g\n", N5, N/N5, N4/N5)
	fmt.Fprintf(w, "Mcascade = %.5g, ACL = %.5g, Kfading = 1/%.5g, Ncascmax = %d\n", s.Mcascade(), s.ACL(), s.Kfading(), s.Ncascmax())
}

func Print_all(w io.Writer, sp *ScanPar, s Counter, rtp, D float64) {
	Print_vi(w, sp, D)
	Print_ci(w, sp, rtp, D)
	Print_spread(w, sp, rtp, D)
	switch stat := s.(type) {
	case *StatGeneric:
		Print_symbols_generic(w, sp, stat, rtp)
	case *StatCascade:
		Print_cascmetrics(w, sp, stat)
		Print_symbols_cascade(w, sp, stat, rtp)
		Print_contribution_falls(w, sp, stat, rtp)
	}
	Print_raw(w, sp, s)
}

// Parsheet for simple slot (without free games and bonuses).
func Parsheet_simple(w io.Writer, sp *ScanPar, s Counter, cost float64) (float64, float64) {
	var µ, D = EvD(s, cost)
	if sp.IsMain() {
		fmt.Fprintf(w, "RTP = %.8g%%\n", µ*100)
	}
	Print_all(w, sp, s, µ, D)
	return µ, D
}

// Parsheet_cascade_simple is for cascade slot games (without free games).
func Parsheet_cascade_simple(w io.Writer, sp *ScanPar, s *StatCascade, cost float64) (float64, float64) {
	var µ, D = EvD(s, cost)
	if sp.IsMain() {
		fmt.Fprintf(w, "RTP = %.8g%%\n", µ*100)
	}
	Print_all(w, sp, s, µ, D)
	return µ, D
}

// Parsheet for slot with retriggerable freegames
// with `m` multiplier on freegames (m=1 if no multiplier).
// Each hit of freegames series has `L` freespins.
func Parsheet_fgretrig(w io.Writer, sp *ScanPar, s Counter, cost, m, L float64) (float64, float64) {
	var µ, Dsym = EvD(s, cost)
	var q, _ = s.FSQ()
	var sq = 1 / (1 - q)
	var Pfg = s.FGQ()
	var rtpfs = m * sq * µ
	var rtp = µ + q*rtpfs
	var Eser, Dser = L * sq, L * q * sq * sq * sq // Galton-Watson process
	var D = Dsym + m*m*Pfg*(Eser*Dsym+µ*µ*Dser)   // Wald's equation
	if sp.IsMain() {
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µ*100, math.Sqrt(Dsym))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/Pfg, q, sq)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µ*100, q, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, s, rtp, D)
	return rtp, D
}

// Parsheet for slot with games series of length L1 with
// free spins series of length L2 that can be triggered only once.
func Parsheet_fgone(w io.Writer, sp *ScanPar, s Counter, cost, m, L1, L2 float64) (float64, float64) {
	var µ, Dsym = EvD(s, cost)
	var Pfg = s.FGQ()                 // P
	var Pre = 1 - math.Pow(1-Pfg, L1) // P(A)=1−(1−P)^N
	var rtp = m * µ * (1 + Pre*L2/L1)
	var Etotal = L1 + L2*Pre
	var Vlen = µ * µ * L2 * L2 * Pre * (1 - Pre)
	var D = m * m * (Etotal*Dsym + Vlen)
	if sp.IsMain() {
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µ*100, math.Sqrt(Dsym))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g\n", 1/Pfg)
		fmt.Fprintf(w, "probability of %g new spins: %.6f\n", L2, Pre)
		fmt.Fprintf(w, "RTP = %.8g%%\n", rtp*100)
	}
	Print_all(w, sp, s, rtp, D)
	return rtp, D
}

func Parsheet_fgretrig_series(w io.Writer, sp *ScanPar, s Counter, cost, m float64, L []int, scat Sym) (float64, float64) {
	var q, _ = s.FSQ()
	return Parsheet_fgretrig_custom(w, sp, s, cost, m,
		q, s.ΣPL(scat, L))
}

func Parsheet_fgretrig_custom(w io.Writer, sp *ScanPar, s Counter, cost, m float64, q, ΣPL float64) (float64, float64) {
	var µ, Dsym = EvD(s, cost)
	var sq = 1 / (1 - q)
	var rtpfs = m * sq * µ
	var rtp = µ + q*rtpfs
	var D = Dsym + m*m*ΣPL*(sq*Dsym+µ*µ*q*sq*sq*sq)
	if sp.IsMain() {
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µ*100, math.Sqrt(Dsym))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/s.FGQ(), q, sq)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µ*100, q, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, s, rtp, D)
	return rtp, D
}

// Parsheet for slot with splitted statistics for regular
// games `sr` and statistics for NON-retriggerable bonus games `sb`.
// with `m` multiplier on freegames (m=1 if no multiplier).
// Each hit of freegames series has `L` freespins.
func Parsheet_fgonce_split(w io.Writer, sp *ScanPar, sr, sb Counter, cost, m, L float64) (float64, float64) {
	// bonus reels parameters
	var µb, Dsymb = EvD(sb, cost)
	// regular reels parameters
	var µr, Dsymr = EvD(sr, cost)
	var qr, _ = sr.FSQ()
	var sqr = 1 / (1 - qr)
	var Pfg = sr.FGQ()
	// calculation
	var rtpfs = m * µb
	var rtp = µr + qr*rtpfs
	var D = Dsymr + m*m*Pfg*(L*Dsymb+L*L*µb*µb)
	if sp.IsFG() {
		fmt.Fprintf(w, "*bonus reels*\n")
		fmt.Fprintf(w, "RTP(fg) = %.8g%%\n", rtpfs*100)
	}
	if sp.IsMain() {
		fmt.Fprintf(w, "*regular reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µr*100, math.Sqrt(Dsymr))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/Pfg, qr, sqr)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µr*100, qr, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, sr, rtp, D)
	return rtp, D
}

// Parsheet for slot with splitted statistics for regular
// games `sr` and statistics for bonus games `sb` in which new games
// can be retriggered only once. Length of first series is `L1`, second is `L2`.
func Parsheet_fgtwice_split(w io.Writer, sp *ScanPar, sr, sb Counter, cost, m, L1, L2 float64) (float64, float64) {
	// bonus reels parameters
	var µb, Dsymb = EvD(sb, cost)
	// regular reels parameters
	var µr, Dsymr = EvD(sr, cost)
	// calculation
	var Pfgb = sb.FGQ()                // P
	var Pre = 1 - math.Pow(1-Pfgb, L1) // P(A)=1−(1−P)^N
	var rtpfs = m * µb * (1 + Pre*L2/L1)
	var qr, _ = sr.FSQ()
	var rtp = µr + qr*rtpfs
	var Etotal = L1 + L2*Pre
	var Vlen = µb * µb * L2 * L2 * Pre * (1 - Pre)
	var Pfgr = sr.FGQ()
	var D = Dsymr + m*m*Pfgr*(Etotal*Dsymb+Vlen) // Wald's equation
	if sp.IsFG() {
		fmt.Fprintf(w, "*bonus reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µb*100, math.Sqrt(Dsymb))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g\n", 1/Pfgb)
		fmt.Fprintf(w, "probability of %g new spins: %.6f\n", L2, Pre)
		fmt.Fprintf(w, "RTP = %.8g%%\n", rtpfs*100)
	}
	if sp.IsMain() {
		fmt.Fprintf(w, "*regular reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µr*100, math.Sqrt(Dsymr))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g\n", 1/Pfgr, qr)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µr*100, qr, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, sr, rtp, D)
	return rtp, D
}

// Parsheet for slot with splitted statistics for regular
// games `sr` and statistics for retriggerable bonus games `sb`.
// with `m` multiplier on freegames (m=1 if no multiplier).
// Each hit of freegames series has `L` freespins.
func Parsheet_fgretrig_split(w io.Writer, sp *ScanPar, sr, sb Counter, cost, m, L float64) (float64, float64) {
	// bonus reels parameters
	var µb, Dsymb = EvD(sb, cost)
	var qb, _ = sb.FSQ()
	var sqb = 1 / (1 - qb)
	// regular reels parameters
	var µr, Dsymr = EvD(sr, cost)
	var qr, _ = sr.FSQ()
	var sqr = 1 / (1 - qr)
	var Pfg = sr.FGQ()
	// calculation
	var rtpfs = m * sqb * µb
	var rtp = µr + qr*rtpfs
	var Eser, Dser = L * sqb, L * qb * sqb * sqb * sqb // Galton-Watson process
	var D = Dsymr + m*m*Pfg*(Eser*Dsymb+µb*µb*Dser)    // Wald's equation
	if sp.IsFG() {
		fmt.Fprintf(w, "*bonus reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µb*100, math.Sqrt(Dsymb))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/sb.FGQ(), qb, sqb)
		fmt.Fprintf(w, "rtp(fg) = m*sq*rtp(sym) = %g*%.5g*%.5g = %.6f%%\n", m, sqb, µb*100, rtpfs*100)
	}
	if sp.IsMain() {
		fmt.Fprintf(w, "*regular reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µr*100, math.Sqrt(Dsymr))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/Pfg, qr, sqr)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µr*100, qr, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, sr, rtp, D)
	return rtp, D
}

func Parsheet_fgretrig_split_series(w io.Writer, sp *ScanPar, sr, sb Counter, cost, m float64, L []int, scat Sym) (float64, float64) {
	var qr, _ = sr.FSQ()
	var qb, _ = sb.FSQ()
	return Parsheet_fgretrig_split_custom(w, sp, sr, sb, cost, m,
		qr, qb, sr.ΣPL(scat, L))
}

func Parsheet_fgretrig_split_custom(w io.Writer, sp *ScanPar, sr, sb Counter, cost, m float64, qr, qb, ΣPL float64) (float64, float64) {
	// bonus reels parameters
	var µb, Dsymb = EvD(sb, cost)
	var sqb = 1 / (1 - qb)
	// regular reels parameters
	var µr, Dsymr = EvD(sr, cost)
	var sqr = 1 / (1 - qr)
	// calculation
	var rtpfs = m * sqb * µb
	var rtp = µr + qr*rtpfs
	var D = Dsymr + m*m*ΣPL*(sqb*Dsymb+µb*µb*qb*sqb*sqb*sqb)
	if sp.IsFG() {
		fmt.Fprintf(w, "*bonus reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µb*100, math.Sqrt(Dsymb))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/sb.FGQ(), qb, sqb)
		fmt.Fprintf(w, "rtp(fg) = m*sq*rtp(sym) = %g*%.5g*%.5g = %.6f%%\n", m, sqb, µb*100, rtpfs*100)
	}
	if sp.IsMain() {
		fmt.Fprintf(w, "*regular reels*\n")
		fmt.Fprintf(w, "symbols: µ = %.8g%%, sigma(sym) = %.6g\n", µr*100, math.Sqrt(Dsymr))
		fmt.Fprintf(w, "free: HRfg = 1/%.5g, q = %.5g, sq = 1/(1-q) = %.5g\n", 1/sr.FGQ(), qr, sqr)
		fmt.Fprintf(w, "RTP = %.5g(sym) + %.5g*%.5g(fg) = %.8g%%\n", µr*100, qr, rtpfs*100, rtp*100)
	}
	Print_all(w, sp, sr, rtp, D)
	return rtp, D
}

// Generic wrapper functions for Parsheet functions.
// These are convenience wrappers that redirect to the underlying Parsheet_* functions.

// Parsheet_generic_simple is a wrapper for Parsheet_simple.
func Parsheet_generic_simple(w io.Writer, sp *ScanPar, s Simulator, cost float64) (float64, float64) {
	return Parsheet_simple(w, sp, s.(Counter), cost)
}

// Parsheet_generic_fgretrig is a wrapper for Parsheet_fgretrig.
func Parsheet_generic_fgretrig(w io.Writer, sp *ScanPar, s Simulator, cost, m, L float64) (float64, float64) {
	return Parsheet_fgretrig(w, sp, s.(Counter), cost, m, L)
}

// Parsheet_generic_fgone is a wrapper for Parsheet_fgone.
func Parsheet_generic_fgone(w io.Writer, sp *ScanPar, s Simulator, cost, m, L1, L2 float64) (float64, float64) {
	return Parsheet_fgone(w, sp, s.(Counter), cost, m, L1, L2)
}

// Parsheet_generic_fgretrig_series is a wrapper for Parsheet_fgretrig_series.
func Parsheet_generic_fgretrig_series(w io.Writer, sp *ScanPar, s Simulator, cost, m float64, L []int, scat Sym) (float64, float64) {
	return Parsheet_fgretrig_series(w, sp, s.(Counter), cost, m, L, scat)
}

// Parsheet_generic_fgonce_split is a wrapper for Parsheet_fgonce_split.
func Parsheet_generic_fgonce_split(w io.Writer, sp *ScanPar, sr, sb Simulator, cost, m, L float64) (float64, float64) {
	return Parsheet_fgonce_split(w, sp, sr.(Counter), sb.(Counter), cost, m, L)
}

// Parsheet_generic_fgtwice_split is a wrapper for Parsheet_fgtwice_split.
func Parsheet_generic_fgtwice_split(w io.Writer, sp *ScanPar, sr, sb Simulator, cost, m, L1, L2 float64) (float64, float64) {
	return Parsheet_fgtwice_split(w, sp, sr.(Counter), sb.(Counter), cost, m, L1, L2)
}

// Parsheet_generic_fgretrig_split is a wrapper for Parsheet_fgretrig_split.
func Parsheet_generic_fgretrig_split(w io.Writer, sp *ScanPar, sr, sb Simulator, cost, m, L float64) (float64, float64) {
	return Parsheet_fgretrig_split(w, sp, sr.(Counter), sb.(Counter), cost, m, L)
}

// Parsheet_generic_fgretrig_split_series is a wrapper for Parsheet_fgretrig_split_series.
func Parsheet_generic_fgretrig_split_series(w io.Writer, sp *ScanPar, sr, sb Simulator, cost, m float64, L []int, scat Sym) (float64, float64) {
	return Parsheet_fgretrig_split_series(w, sp, sr.(Counter), sb.(Counter), cost, m, L, scat)
}
