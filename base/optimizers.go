package base

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/optimize"
)

// Optimizer has updateParams method to update theta from gradient
type Optimizer interface {
	GetUpdate(update *mat.Dense, grad mat.Matrix)
	UpdateParams(grad mat.Matrix)
	SetTheta(Theta *mat.Dense)
	GetTheta() *mat.Dense
	GetTimeStep() uint64
	String() string
}

// SGDOptimizer is struct for SGD solver v https://en.wikipedia.org/wiki/Stochastic_gradient_descent
type SGDOptimizer struct {
	// StepSize is used for all variants
	// Momentum can be used for all variants
	// GradientClipping is used if >0 to limit gradient L2 norm
	// RMSPropGamma is the momentum for rmsprop and adadelta
	// Epsilon is used to avoid division by zero in adagrad,rmsprop,adadelta,adam
	StepSize, Momentum, GradientClipping, RMSPropGamma, Epsilon float64
	// Adagrad, Adadelta, RMSProp, Adam are variants. At most one should be true
	Adagrad, Adadelta, RMSProp, Adam bool
	// NFeature,NOutputs need only to be initialized wher SGDOptimizer is used as an optimize.Method
	NFeatures, NOutputs int

	// running Parameters (don't set them yourself)
	GtNorm, Theta, PrevUpdate, Update, AdagradG, AdadeltaU *mat.Dense
	TimeStep                                               float64
	// Adam specific
	Beta1, Beta2         float64
	Mt, Vt, Mtcap, Vtcap *mat.Dense
	// lastOp is for Iterate when used as optimize.Method
	lastOp optimize.Operation
}

// NewSGDOptimizer returns an initialized *SGDOptimizer with stepsize 1e-4 and momentum 0.9
func NewSGDOptimizer() *SGDOptimizer {
	s := &SGDOptimizer{StepSize: 1e-4, Momentum: .9, RMSPropGamma: .9, Epsilon: 1e-8}

	return s
}

// NewAdagradOptimizer return a *SGDOptimizer setup for adagrad
func NewAdagradOptimizer() *SGDOptimizer {
	s := NewSGDOptimizer()
	s.StepSize = .5
	s.Momentum = 0.
	s.Adagrad = true
	s.GradientClipping = 10.
	return s
}

// NewAdadeltaOptimizer return a *SGDOptimizer setup for adadelta
func NewAdadeltaOptimizer() *SGDOptimizer {
	s := NewSGDOptimizer()
	s.Momentum = 0.
	s.Adadelta = true
	return s
}

// NewRMSPropOptimizer return a *SGDOptimizer setup for rmsprop
func NewRMSPropOptimizer() *SGDOptimizer {
	s := NewSGDOptimizer()
	s.StepSize = 0.05
	s.Momentum = 0.
	s.RMSProp = true
	s.RMSPropGamma = 0.9
	return s
}

// NewAdamOptimizer returns an initialized adam solver
func NewAdamOptimizer() *SGDOptimizer {
	s := &SGDOptimizer{StepSize: .5, Beta1: .9, Beta2: .999, Epsilon: 1e-8, Adam: true}
	return s
}

func (s *SGDOptimizer) String() string {
	switch {
	case s.Adagrad:
		return "adagrad"
	case s.RMSProp:
		return "rmsprop" + fmt.Sprintf(" gamma:%g", s.RMSPropGamma)
	case s.Adadelta:
		return "adadelta" + fmt.Sprintf(" gamma:%g", s.RMSPropGamma)
	case s.Adam:
		return "adam"
	default:
		return "sgd" + fmt.Sprintf(" StepSize:%g,Momentum:%g", s.StepSize, s.Momentum)
	}

}

// NewOptimizer only accepts SGD|adagrad|adadelta|rmsprop|adam
func NewOptimizer(name string) Optimizer {
	switch name {
	case "sgd":
		return NewSGDOptimizer()
	case "adagrad":
		return NewAdagradOptimizer()
	case "adadelta":
		return NewAdadeltaOptimizer()
	case "rmsprop":
		return NewRMSPropOptimizer()
	case "adam":
		return NewAdamOptimizer()
	default:
		panic("NewOptimizer only accepts SGD|adagrad|adadelta|rmsprop|adam")
	}
}

// SetTheta should be called before first call to UpdateParams to let the solver know the theta pointer
func (s *SGDOptimizer) SetTheta(Theta *mat.Dense) {
	s.NFeatures, s.NOutputs = Theta.Dims()
	s.Theta = Theta
}

// GetTheta can be called anytime after SetTheta to get read access to theta
func (s *SGDOptimizer) GetTheta() *mat.Dense { return s.Theta }

// GetTimeStep return the number of theta updates already occurred
func (s *SGDOptimizer) GetTimeStep() uint64 { return uint64(s.TimeStep) }

// UpdateParams updates theta from gradient. first call allocates required temporary storage
func (s *SGDOptimizer) UpdateParams(grad mat.Matrix) {
	r, c := grad.Dims()
	if s.Update == nil {
		s.Update = mat.NewDense(r, c, nil)
	}
	s.GetUpdate(s.Update, grad)
	s.Theta.Add(s.Theta, s.Update)
}

// GetUpdate compute the update from grad
func (s *SGDOptimizer) GetUpdate(update *mat.Dense, grad mat.Matrix) {
	NFeatures, NOutputs := grad.Dims()
	if s.TimeStep == 0. {
		init := func(m *mat.Dense, v0 float64) *mat.Dense {
			m.Apply(func(i int, j int, v float64) float64 { return v0 }, m)
			return m
		}
		if s.GradientClipping > 0. {
			s.GtNorm = mat.NewDense(NOutputs, 1, nil)
		}
		s.PrevUpdate = mat.NewDense(NFeatures, NOutputs, nil)
		if s.Adagrad || s.RMSProp || s.Adadelta {
			s.AdagradG = init(mat.NewDense(NFeatures, NOutputs, nil), s.Epsilon)
		}
		if s.Adadelta {
			s.AdadeltaU = init(mat.NewDense(NFeatures, NOutputs, nil), 1.)
		}
		if s.Adam {
			s.Mt = mat.NewDense(NFeatures, NOutputs, nil)
			s.Vt = mat.NewDense(NFeatures, NOutputs, nil)
			s.Mtcap = mat.NewDense(NFeatures, NOutputs, nil)
			s.Vtcap = mat.NewDense(NFeatures, NOutputs, nil)
		}
	}
	s.TimeStep += 1.
	// gt ← ∇θft(θt−1) (Get gradients w.r.t. stochastic objective at timestep t)

	eta := s.StepSize * 100. / (100. + s.TimeStep)
	if s.GradientClipping > 0. {
		for j := 0; j < NOutputs; j++ {
			s.GtNorm.Set(j, 0, colNorm(grad, j))
		}
	}
	gradientClipped := func(j, o int) float64 {
		gradjo := grad.At(j, o)
		if s.GradientClipping > 0. && s.GtNorm.At(o, 0) > s.GradientClipping {
			gradjo *= s.GradientClipping / s.GtNorm.At(o, 0)
		}
		return gradjo
	}

	// Compute S.Update

	if s.RMSProp {
		update.Apply(func(j, o int, v float64) float64 {
			etajo := s.StepSize
			if s.TimeStep > 1 && math.Abs(s.AdagradG.At(j, o)) > 1. {
				etajo /= math.Sqrt(s.AdagradG.At(j, o) + s.Epsilon)
			}
			return -etajo * gradientClipped(j, o)

		}, s.AdagradG)
		s.AdagradG.Apply(func(j, o int, v float64) float64 {
			gradjo := gradientClipped(j, o)
			v = v*s.RMSPropGamma + (1.-s.RMSPropGamma)*gradjo*gradjo
			return v
		}, s.AdagradG)
	} else if s.Adagrad {
		update.Apply(func(j, o int, v float64) float64 {
			etajo := s.StepSize
			Gjo := s.AdagradG.At(j, o)
			if s.TimeStep > 1 {
				etajo /= math.Sqrt(Gjo) + s.Epsilon
			}
			return -etajo * gradientClipped(j, o)
		}, grad)
		// accumulate gradients
		s.AdagradG.Apply(func(j, o int, v float64) float64 {
			gradjo := gradientClipped(j, o)
			v += gradjo * gradjo
			return v
		}, s.AdagradG)
	} else if s.Adadelta {
		// https://arxiv.org/pdf/1212.5701.pdf
		// accumulate gradients
		s.AdagradG.Apply(func(j, o int, v float64) float64 {
			gradjo := gradientClipped(j, o)
			return s.RMSPropGamma*v + (1.-s.RMSPropGamma)*gradjo*gradjo
		}, s.AdagradG)
		// compute update
		update.Apply(func(j, o int, gradjo float64) float64 {
			etajo := eta
			if s.TimeStep > 1 {
				etajo = math.Sqrt(s.AdadeltaU.At(j, o)) / math.Sqrt(s.AdagradG.At(j, o)+s.Epsilon)
			}
			return -etajo * gradientClipped(j, o)
		}, grad)
		s.AdadeltaU.Apply(func(j, o int, v float64) float64 {
			upd := update.At(j, o)
			return s.RMSPropGamma*v + (1.-s.RMSPropGamma)*upd*upd
		}, s.AdadeltaU)
	} else if s.Adam {
		// mt ← β1 · mt−1 + (1 − β1) · gt (Update biased first moment estimate)

		s.Mt.Apply(func(j, o int, gradjo float64) float64 {
			return s.Beta1*s.Mt.At(j, o) + (1.-s.Beta1)*gradientClipped(j, o)
		}, grad)
		// vt ← β2 · vt−1 + (1 − β2) · gt² (Update biased second raw moment estimate)
		s.Vt.Apply(func(j, o int, gradjo float64) float64 {
			gradjo = gradientClipped(j, o)
			return s.Beta2*s.Vt.At(j, o) + (1.-s.Beta2)*gradjo*gradjo
		}, grad)
		// mb t ← mt/(1 − β1^t) (Compute bias-corrected first moment estimate)
		s.Mtcap.Scale(1./(1.-math.Pow(s.Beta1, s.TimeStep)), s.Mt)
		// vbt ← vt/(1 − β2^t) (Compute bias-corrected second raw moment estimate)
		s.Vtcap.Scale(1./(1.-math.Pow(s.Beta2, s.TimeStep)), s.Vt)
		// θt ← θt−1 − α · mb t/(√vbt + epsilon) (Update parameters)

		update.Apply(func(i, j int, Mtcapij float64) float64 {
			return -s.StepSize * Mtcapij / (math.Sqrt(s.Vtcap.At(i, j)) + s.Epsilon)
		}, s.Mtcap)
	} else {
		// normal SGD with momentum
		update.Apply(func(j, o int, gradjo float64) float64 {
			return -eta * gradientClipped(j, o) / math.Sqrt(1.*s.TimeStep)
		}, grad)
	}
	// Apply Momentum
	if s.Momentum > 0 {
		update.Apply(func(j, o int, updjo float64) float64 {
			return s.Momentum*s.PrevUpdate.At(j, o) + updjo
		}, update)
	}
	s.PrevUpdate.Clone(update)
}

// colNorm returns the L2 norm of a matrix column
func colNorm(m mat.Matrix, o int) float64 {
	nFeatures, _ := m.Dims()
	s := 0.
	for j := 0; j < nFeatures; j++ {
		v := m.At(j, o)
		s += v * v
	}
	return math.Sqrt(s)
}

// Init initializes the method based on the initial data in loc, updates it
// and returns the first operation to be carried out by the caller.
// The initial location must be valid as specified by Needs.
func (s *SGDOptimizer) Init(loc *optimize.Location) (op optimize.Operation, err error) {
	//fmt.Println("SGDOptimizer.Init called with len(loc.X)=", len(loc.X))
	if s.NFeatures == 0 || s.NOutputs == 0 {
		s.NFeatures = len(loc.X)
		return
	}
	if len(loc.X) == s.NFeatures {
		s.NOutputs = 1
	}
	if len(loc.X) != s.NFeatures*s.NOutputs {
		err = fmt.Errorf("Size error. expected %d,%d got %d", s.NFeatures, s.NOutputs, len(loc.X))
		return
	}
	s.Update = mat.NewDense(s.NFeatures, s.NOutputs, nil)
	op = optimize.FuncEvaluation | optimize.GradEvaluation
	return
}

// Iterate retrieves data from loc, performs one iteration of the method,
// updates loc and returns the next operation.
func (s *SGDOptimizer) Iterate(loc *optimize.Location) (op optimize.Operation, err error) {
	theta := mat.NewDense(s.NFeatures, s.NOutputs, loc.X)

	s.GetUpdate(s.Update, mat.NewDense(s.NFeatures, s.NOutputs, loc.Gradient))
	theta.Add(theta, s.Update)
	//op = optimize.FuncEvaluation | optimize.GradEvaluation
	if s.lastOp == optimize.FuncEvaluation|optimize.GradEvaluation {
		op = optimize.MajorIteration
	} else {
		op = optimize.FuncEvaluation | optimize.GradEvaluation
	}
	s.lastOp = op
	return
}

// Needs is for when SGDOptimizer is used as an optimize.Method
func (*SGDOptimizer) Needs() struct {
	Gradient bool
	Hessian  bool
} {
	return struct {
		Gradient bool
		Hessian  bool
	}{
		Gradient: true,
		Hessian:  false,
	}
}

type matDense struct {
	*mat.Dense
	data []float64
}

func matDenseNew(r, c int, data []float64) *matDense {
	if data != nil && r*c != len(data) {
		panic("ErrShape")
	}
	if data == nil {
		data = make([]float64, r*c)
	}
	return &matDense{mat.NewDense(r, c, data), data}
}
func (m *matDense) Dims() (r, c int)        { r, c = m.Dense.Dims(); return }
func (m *matDense) AT(i, j int) float64     { return m.Dense.At(i, j) }
func (m *matDense) T() mat.Matrix           { return m.Dense.T() }
func (m *matDense) Set(i, j int, v float64) { m.Dense.Set(i, j, v) }