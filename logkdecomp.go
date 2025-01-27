//Log-K-Decomp  -  A research prototype to compute Hypertree Decompositions of
// Conjunctive Queries and Constraint Satisfaction Problems, using a parallel algorithm
// with logarithmic recursion depth.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/cem-okulmus/BalancedGo/lib"
)

// Algorithm serves as the common interface of all hypergraph decomposition algorithms
type Algorithm interface {
	// A Name is useful to identify the individual algorithms in the result
	Name() string
	FindDecomp() lib.Decomp
	FindDecompGraph(G lib.Graph) lib.Decomp
	SetWidth(K int)
}

// Decomp used to improve readability
type Decomp = lib.Decomp

// Edge used to improve readability
type Edge = lib.Edge

// Graph used to improve readability
type Graph = lib.Graph

func logActive(b bool) {
	if b {
		log.SetOutput(os.Stderr)

		log.SetFlags(0)
	} else {

		log.SetOutput(ioutil.Discard)
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}

}

type labelTime struct {
	time  float64
	label string
}

func (l labelTime) String() string {
	return fmt.Sprintf("%s : %.5f ms", l.label, l.time)
}

func outputStanza(algorithm string, decomp Decomp, times []labelTime, graph Graph, gml string, K int, skipCheck bool) {
	decomp.RestoreSubedges()

	fmt.Println("Used algorithm: " + algorithm)
	fmt.Println("Result ( ran with K =", K, ")\n", decomp)

	// Print the times
	var sumTotal float64

	for _, time := range times {
		sumTotal = sumTotal + time.time
	}
	fmt.Printf("Time: %.5f ms\n", sumTotal)

	fmt.Println("Time Composition: ")
	for _, time := range times {
		fmt.Println(time)
	}

	fmt.Println("\nWidth: ", decomp.CheckWidth())
	var correct bool
	if !skipCheck {
		correct = decomp.Correct(graph)
	} else {
		correct = true
	}

	fmt.Println("Correct: ", correct)
	if correct && len(gml) > 0 {
		f, err := os.Create(gml)
		check(err)

		defer f.Close()
		f.WriteString(decomp.ToGML())
		f.Sync()
	}
}

func main() {

	// ==============================================
	// Command-Line Argument Parsing

	flagSet := flag.NewFlagSet("", flag.ContinueOnError)
	flagSet.SetOutput(ioutil.Discard)

	// input flags
	graphPath := flagSet.String("graph", "", "input (for format see hyperbench.dbai.tuwien.ac.at/downloads/manual.pdf)")
	width := flagSet.Int("width", 0, "a positive, non-zero integer indicating the width of the HD to search for")
	exact := flagSet.Bool("exact", false, "Compute exact width (width flag ignored)")
	approx := flagSet.Int("approx", 0, "Compute approximated width and set a timeout in seconds (width flag ignored)")

	// algorithms  flags
	logK := flagSet.Bool("logk", false, "Use LogKDecomp algorithm")
	logKHybrid := flagSet.Int("logkHybrid", 0, "Use DetK - LogK Hybrid algorithm. Choose which predicate to use")

	// heuristic flags
	heur := "1 ... Vertex Degree Ordering\n\t2 ... Max. Separator Ordering\n\t3 ... MCSO\n\t4 ... Edge Degree Ordering"
	useHeuristic := flagSet.Int("heuristic", 0, "turn on to activate edge ordering\n\t"+heur)
	gyö := flagSet.Bool("g", false, "perform a GYÖ reduct")
	typeC := flagSet.Bool("t", false, "perform a Type Collapse")
	hingeFlag := flagSet.Bool("h", false, "use hingeTree Optimization")

	//other optional  flags
	cpuprofile := flagSet.String("cpuprofile", "", "write cpu profile to file")
	logging := flagSet.Bool("log", false, "turn on extensive logs")
	balanceFactorFlag := flagSet.Int("balfactor", 2, "Changes the factor that balanced separator check uses, default 2")
	numCPUs := flagSet.Int("cpu", -1, "Set number of CPUs to use")
	bench := flagSet.Bool("bench", false, "Benchmark mode, reduces unneeded output (incompatible with -log flag)")
	gml := flagSet.String("gml", "", "Output the produced decomposition into the specified gml file ")
	pace := flagSet.Bool("pace", false, "Use PACE 2019 format for graphs (see pacechallenge.org/2019/htd/htd_format/)")
	meta := flagSet.Int("meta", 0, "meta parameter for LogKHybrid")

	parseError := flagSet.Parse(os.Args[1:])
	if parseError != nil {
		fmt.Print("Parse Error:\n", parseError.Error(), "\n\n")
	}

	// Output usage message if graph and width not specified
	if parseError != nil || *graphPath == "" || (*width <= 0 && !*exact && *approx == 0) {
		out := fmt.Sprint("Usage of log-k-decomp:")
		fmt.Fprintln(os.Stderr, out)
		flagSet.VisitAll(func(f *flag.Flag) {
			if f.Name != "width" && f.Name != "graph" && f.Name != "exact" && f.Name != "approx" {
				return
			}
			s := fmt.Sprintf("%T", f.Value) // used to get type of flag
			if s[6:len(s)-5] != "bool" {
				fmt.Printf("  -%-10s \t<%s>\n", f.Name, s[6:len(s)-5])
			} else {
				fmt.Printf("  -%-10s \n", f.Name)
			}
			fmt.Println("\t" + f.Usage)
		})

		fmt.Println("\nAlgorithm Choice: ")
		flagSet.VisitAll(func(f *flag.Flag) {
			if f.Name != "logk" && f.Name != "logkHybrid" {
				return
			}
			s := fmt.Sprintf("%T", f.Value) // used to get type of flag
			if s[6:len(s)-5] != "bool" {
				fmt.Printf("  -%-10s \t<%s>\n", f.Name, s[6:len(s)-5])
			} else {
				fmt.Printf("  -%-10s \n", f.Name)
			}
			fmt.Println("\t" + f.Usage)
		})

		fmt.Println("\nOptional Arguments: ")
		flagSet.VisitAll(func(f *flag.Flag) {
			if f.Name == "width" || f.Name == "graph" || f.Name == "exact" || f.Name == "approx" || f.Name == "logkHybrid" || f.Name == "logk" {
				return
			}
			s := fmt.Sprintf("%T", f.Value) // used to get type of flag
			if s[6:len(s)-5] != "bool" {
				fmt.Printf("  -%-10s \t<%s>\n", f.Name, s[6:len(s)-5])
			} else {
				fmt.Printf("  -%-10s \n", f.Name)
			}
			fmt.Println("\t" + f.Usage)
		})

		return
	}

	// END Command-Line Argument Parsing
	// ==============================================

	if *exact && (*approx > 0) {
		fmt.Println("Cannot have exact and approx flags set at the same time. Make up your mind.")
		return
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)

		defer pprof.StopCPUProfile()
	}

	if *bench { // no logging output when running benchmarks
		*logging = false
	}
	logActive(*logging)

	BalFactor := *balanceFactorFlag

	runtime.GOMAXPROCS(*numCPUs)

	dat, err := ioutil.ReadFile(*graphPath)
	check(err)

	var parsedGraph Graph

	if !*pace {
		parsedGraph, _ = lib.GetGraph(string(dat))
	} else {
		parsedGraph = lib.GetGraphPACE(string(dat))
	}

	originalGraph := parsedGraph

	if !*bench { // skip any output if bench flag is set
		log.Println("BIP: ", parsedGraph.GetBIP())
	}

	var reducedGraph Graph

	var times []labelTime

	// Sorting Edges to find separators faster
	if *useHeuristic > 0 {
		var heuristicMessage string

		start := time.Now()
		switch *useHeuristic {
		case 1:
			parsedGraph.Edges = lib.GetDegreeOrder(parsedGraph.Edges)
			heuristicMessage = "Using degree ordering as a heuristic"
			break
		case 2:
			parsedGraph.Edges = lib.GetMaxSepOrder(parsedGraph.Edges)
			heuristicMessage = "Using max separator ordering as a heuristic"
			break
		case 3:
			parsedGraph.Edges = lib.GetMSCOrder(parsedGraph.Edges)
			heuristicMessage = "Using MSC ordering as a heuristic"
			break
		case 4:
			parsedGraph.Edges = lib.GetEdgeDegreeOrder(parsedGraph.Edges)
			heuristicMessage = "Using edge degree ordering as a heuristic"
			break
		}
		d := time.Now().Sub(start)
		msec := d.Seconds() * float64(time.Second/time.Millisecond)
		times = append(times, labelTime{time: msec, label: "Heuristic"})

		if !*bench {
			fmt.Println(heuristicMessage)
			fmt.Printf("Time for heuristic: %.5f ms\n", msec)
			fmt.Printf("Ordering: %v\n", parsedGraph.String())
		}
	}
	var removalMap map[int][]int
	// Performing Type Collapse
	if *typeC {
		count := 0
		reducedGraph, removalMap, count = parsedGraph.TypeCollapse()
		parsedGraph = reducedGraph
		if !*bench { // be silent when benchmarking
			fmt.Println("\n\n", *graphPath)
			fmt.Println("Graph after Type Collapse:")
			for _, e := range reducedGraph.Edges.Slice() {
				fmt.Printf("%v %v\n", e, Edge{Vertices: e.Vertices})
			}
			fmt.Print("Removed ", count, " vertex/vertices\n\n")
		}
	}

	var ops []lib.GYÖReduct
	// Performing GYÖ reduction
	if *gyö {

		if *typeC {
			reducedGraph, ops = reducedGraph.GYÖReduct()
		} else {
			reducedGraph, ops = parsedGraph.GYÖReduct()
		}

		parsedGraph = reducedGraph
		if !*bench { // be silent when benchmarking
			fmt.Println("Graph after GYÖ:")
			fmt.Println(reducedGraph)
			fmt.Println("Reductions:")
			fmt.Print(ops, "\n\n")
		}

	}

	var hinget lib.Hingetree
	var msecHinge float64

	if *hingeFlag {
		startHinge := time.Now()

		hinget = lib.GetHingeTree(parsedGraph)

		dHinge := time.Now().Sub(startHinge)
		msecHinge = dHinge.Seconds() * float64(time.Second/time.Millisecond)
		times = append(times, labelTime{time: msecHinge, label: "Hingetree"})

		if !*bench {
			fmt.Println("Produced Hingetree: ")
			fmt.Println(hinget)
		}
	}

	var solver Algorithm

	// Check for multiple flags
	chosen := 0

	if *logK {
		logK := LogKDecomp{
			Graph:     parsedGraph,
			K:         *width,
			BalFactor: BalFactor,
		}
		solver = &logK
		chosen++
	}

	if *logKHybrid > 0 {
		logKHyb := LogKHybrid{
			Graph:     parsedGraph,
			K:         *width,
			BalFactor: BalFactor,
		}
		logKHyb.Size = *meta

		var pred HybridPredicate

		switch *logKHybrid {
		case 1:
			pred = logKHyb.NumberEdgesPred
		case 2:
			pred = logKHyb.SumEdgesPred
		case 3:
			pred = logKHyb.ETimesKDivAvgEdgePred
		case 4:
			pred = logKHyb.OneRoundPred

		}

		logKHyb.Predicate = pred // set the predicate to use

		solver = &logKHyb
		chosen++
	}

	if chosen > 1 {
		fmt.Println("Only one algorithm may be chosen at a time. Make up your mind.")
		return
	}

	if solver != nil {

		var decomp Decomp
		start := time.Now()

		if *hingeFlag {
			decomp = hinget.DecompHinge(solver, parsedGraph)
		} else {
			decomp = solver.FindDecomp()
		}

		d := time.Now().Sub(start)
		msec := d.Seconds() * float64(time.Second/time.Millisecond)
		times = append(times, labelTime{time: msec, label: "Decomposition"})

		if !reflect.DeepEqual(decomp, Decomp{}) || (len(ops) > 0 && parsedGraph.Edges.Len() == 0) {
			var result bool
			decomp.Root, result = decomp.Root.RestoreGYÖ(ops)
			if !result {
				fmt.Println("Partial decomp:", decomp.Root)
				log.Panicln("GYÖ reduction failed")
			}
			decomp.Root, result = decomp.Root.RestoreTypes(removalMap)
			if !result {
				fmt.Println("Partial decomp:", decomp.Root)
				log.Panicln("Type Collapse reduction failed")
			}
		}

		if !reflect.DeepEqual(decomp, Decomp{}) {
			decomp.Graph = originalGraph
		}
		outputStanza(solver.Name(), decomp, times, originalGraph, *gml, *width, false)

		return
	}

	fmt.Println("No algorithm or procedure selected.")
}
