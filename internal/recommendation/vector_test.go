package recommendation

import (
	"math"
	"testing"
)

func TestHashBookFeatures(t *testing.T) {
	v := HashBookFeatures(
		[]string{"Author One", "Author Two"},
		[]string{"My Series"},
		[]string{"Fiction", "Sci-Fi"},
		[]string{"tag1", "tag2"},
		"en",
		"Publisher",
	)

	// Vector should be non-zero.
	allZero := true
	for _, x := range v {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("expected non-zero vector")
	}

	// Vector should be normalized (L2 norm ~= 1).
	var normSq float64
	for _, x := range v {
		normSq += float64(x) * float64(x)
	}
	norm := math.Sqrt(normSq)
	if math.Abs(norm-1.0) > 1e-5 {
		t.Errorf("expected norm ~1, got %f", norm)
	}
}

func TestNormalizeVector(t *testing.T) {
	v := [vectorDim]float32{3, 4}
	nv := NormalizeVector(v)

	var normSq float64
	for _, x := range nv {
		normSq += float64(x) * float64(x)
	}
	norm := math.Sqrt(normSq)
	if math.Abs(norm-1.0) > 1e-5 {
		t.Errorf("expected norm ~1, got %f", norm)
	}

	// First component should be 3/5, second 4/5.
	if math.Abs(float64(nv[0])-0.6) > 1e-5 {
		t.Errorf("expected nv[0] ~0.6, got %f", nv[0])
	}
	if math.Abs(float64(nv[1])-0.8) > 1e-5 {
		t.Errorf("expected nv[1] ~0.8, got %f", nv[1])
	}
}

func TestNormalizeVectorZero(t *testing.T) {
	v := [vectorDim]float32{}
	nv := NormalizeVector(v)
	for i, x := range nv {
		if x != 0 {
			t.Errorf("expected zero at index %d, got %f", i, x)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := [vectorDim]float32{1, 0, 0}
	b := [vectorDim]float32{1, 0, 0}
	if sim := CosineSimilarity(a, b); math.Abs(float64(sim)-1.0) > 1e-5 {
		t.Errorf("expected similarity 1 for identical vectors, got %f", sim)
	}

	c := [vectorDim]float32{0, 1, 0}
	if sim := CosineSimilarity(a, c); math.Abs(float64(sim)-0.0) > 1e-5 {
		t.Errorf("expected similarity 0 for orthogonal vectors, got %f", sim)
	}
}

func TestVectorToBytesAndBack(t *testing.T) {
	v := [vectorDim]float32{}
	v[0] = 1.5
	v[1] = -2.25
	v[127] = 0.75

	b := vectorToBytes(v)
	if len(b) != vectorDim*4 {
		t.Fatalf("expected %d bytes, got %d", vectorDim*4, len(b))
	}

	vv := vectorFromBytes(b)
	for i := range v {
		if v[i] != vv[i] {
			t.Errorf("mismatch at index %d: expected %f, got %f", i, v[i], vv[i])
		}
	}
}

func TestVectorFromBytesShort(t *testing.T) {
	v := vectorFromBytes([]byte{1, 2, 3})
	for i, x := range v {
		if x != 0 {
			t.Errorf("expected zero at index %d for short input, got %f", i, x)
		}
	}
}
