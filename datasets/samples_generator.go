package datasets

import (
	"math/rand"
	"sort"

	"github.com/gcla/sklearn/base"
	"gonum.org/v1/gonum/mat"
)

// MakeRegression Generate a random regression problem
// n_samples : int, optional (default=100) The number of samples.
// n_features : int, optional (default=100)  The number of features.
// n_informative : int, optional (default=10)  The number of informative features, i.e., the number of features used to build the linear model used to generate the output.
// n_targets : int, optional (default=1) The number of regression targets, i.e., the dimension of the y output vector associated with a sample. By default, the output is a scalar.
// bias : float64 or []float64 or mat.Matrix, optional (default=0.0) The bias term in the underlying linear model.
// effective_rank : int , optional (default=None) currently unused
// tail_strength : float between 0.0 and 1.0, optional (default=0.5) currently unused
// shuffle : boolean, optional (default=True)
// coef : boolean. the coefficients of the underlying linear model are returned regardless its value.
// random_state : *math.Rand optional (default=nil)
func MakeRegression(kwargs map[string]interface{}) (X, y, Coef *mat.Dense) {
	rnd := func() float64 { return rand.NormFloat64() }
	var nSamples, nFeatures, nInformative, nTargets, Shuffle = 100, 100, 10, 1, true
	if v, ok := kwargs["n_samples"]; ok {
		nSamples = v.(int)
	}
	if v, ok := kwargs["n_features"]; ok {
		nFeatures = v.(int)
	}
	if v, ok := kwargs["n_informative"]; ok {
		nInformative = v.(int)
	}
	if v, ok := kwargs["n_targets"]; ok {
		nTargets = v.(int)
	}
	if v, ok := kwargs["random_state"]; ok {
		rnd = func() float64 { return v.(*rand.Rand).NormFloat64() }
	}
	X = mat.NewDense(nSamples, nFeatures, nil)
	if !Shuffle {
		col := make([]float64, nSamples)
		for feat := 0; feat < nFeatures; feat++ {
			mat.Col(col, feat, X)
			sort.Float64s(col)
			X.SetCol(feat, col)
		}

	}
	y = mat.NewDense(nSamples, nTargets, nil)
	if nInformative > nFeatures {
		nInformative = nFeatures
	}
	Coef = mat.NewDense(nInformative, nTargets, nil)
	xmat := X.RawMatrix()
	for xi := 0; xi < xmat.Rows*xmat.Stride; xi += xmat.Stride {
		for xj := 0; xj < xmat.Cols; xj++ {
			xmat.Data[xi+xj] = rnd()
		}
	}
	cmat := Coef.RawMatrix()
	for ci := 0; ci < cmat.Rows*cmat.Stride; ci += cmat.Stride {
		for cj := 0; cj < cmat.Cols; cj++ {
			cmat.Data[ci+cj] = rnd()
		}
	}
	base.MatDimsCheck(".", y, X.Slice(0, nSamples, 0, nInformative), Coef)
	y.Mul(X.Slice(0, nSamples, 0, nInformative), Coef)
	if v, ok := kwargs["bias"]; ok {
		ymat := y.RawMatrix()
		switch vv := v.(type) {
		case float64:
			for yi := 0; yi < ymat.Rows*ymat.Stride; yi += ymat.Stride {
				for yj := 0; yj < ymat.Cols; yj++ {
					ymat.Data[yi+yj] += vv
				}
			}
		case mat.Matrix:
			for yi := 0; yi < ymat.Rows*ymat.Stride; yi += ymat.Stride {
				for yj := 0; yj < ymat.Cols; yj++ {
					ymat.Data[yi+yj] += vv.At(0, yj)
				}
			}
		case []float64:
			for yi := 0; yi < ymat.Rows*ymat.Stride; yi += ymat.Stride {
				for yj := 0; yj < ymat.Cols; yj++ {
					ymat.Data[yi+yj] += vv[yj]
				}
			}
		}

	}
	return
}

// sklearn.datasets.make_classification(n_samples=100, n_features=20, n_informative=2, n_redundant=2, n_repeated=0, n_classes=2, n_clusters_per_class=2, weights=None, flip_y=0.01, class_sep=1.0, hypercube=True, shift=0.0, scale=1.0, shuffle=True, random_state=None)[source]
