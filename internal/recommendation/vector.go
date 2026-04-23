package recommendation

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// feature weights for vector hashing.
const (
	weightAuthors    = 0.30
	weightSeries     = 0.20
	weightCategories = 0.20
	weightTags       = 0.15
	weightLanguage   = 0.10
	weightPublisher  = 0.05
)

// vectorDim is the fixed dimensionality of book feature vectors.
const vectorDim = 128

// HashBookFeatures computes a feature-hashed vector for a book using FNV-1a.
// Features are hashed into a 128-dimensional float32 vector with per-feature
// weights, then L2-normalized.
func HashBookFeatures(authors, series, categories, tags []string, language, publisher string) [vectorDim]float32 {
	var v [vectorDim]float32

	for _, a := range authors {
		hashFeature(&v, a, weightAuthors)
	}
	for _, s := range series {
		hashFeature(&v, s, weightSeries)
	}
	for _, c := range categories {
		hashFeature(&v, c, weightCategories)
	}
	for _, t := range tags {
		hashFeature(&v, t, weightTags)
	}
	if language != "" {
		hashFeature(&v, language, weightLanguage)
	}
	if publisher != "" {
		hashFeature(&v, publisher, weightPublisher)
	}

	return NormalizeVector(v)
}

// hashFeature adds a single feature into the vector using FNV-1a hashing.
func hashFeature(v *[vectorDim]float32, s string, weight float32) {
	h := fnv64a(s)
	sign := float32(1)
	if h&1 != 0 {
		sign = -1
	}
	idx := (h >> 1) % vectorDim
	v[idx] += sign * weight
}

// fnv64a returns the FNV-1a 64-bit hash of s.
func fnv64a(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// NormalizeVector returns an L2-normalized copy of v.
// If the norm is zero, the zero vector is returned.
func NormalizeVector(v [vectorDim]float32) [vectorDim]float32 {
	var normSq float64
	for _, x := range v {
		normSq += float64(x) * float64(x)
	}
	if normSq == 0 {
		return v
	}
	norm := float32(math.Sqrt(normSq))
	for i := range v {
		v[i] /= norm
	}
	return v
}

// CosineSimilarity returns the cosine similarity between two vectors.
// Both vectors are assumed to be L2-normalized; the result is the dot product.
func CosineSimilarity(a, b [vectorDim]float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// vectorToBytes serializes a [128]float32 vector to a 512-byte slice.
func vectorToBytes(v [vectorDim]float32) []byte {
	b := make([]byte, vectorDim*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// vectorFromBytes deserializes a 512-byte slice to a [128]float32 vector.
func vectorFromBytes(b []byte) [vectorDim]float32 {
	var v [vectorDim]float32
	if len(b) < vectorDim*4 {
		return v
	}
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
