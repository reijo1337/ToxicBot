package message

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allPhotoAngles() []string {
	out := append([]string{}, photoImageAngles...)
	return append(out, photoCaptionAngle, photoForwardAngle)
}

func TestBuildPhotoSteering_WithAllAngles(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	angles := allPhotoAngles()

	var sawFree, sawAngle bool
	for range 2000 {
		out := BuildPhotoSteering(r, true, true)
		require.Contains(t, out, photoSteeringAntiTemplate,
			"в каждой подсказке должно быть анти-шаблонное правило")

		angleCount := 0
		for _, a := range angles {
			if strings.Contains(out, a) {
				angleCount++
			}
		}
		if out == photoSteeringAntiTemplate {
			sawFree = true
			assert.Equal(t, 0, angleCount, "в свободном режиме углов быть не должно")
		} else {
			sawAngle = true
			assert.Equal(t, 1, angleCount, "в режиме с углом должен быть ровно один угол")
		}
	}
	assert.True(t, sawFree, "за 2000 итераций должен встретиться свободный режим")
	assert.True(t, sawAngle, "за 2000 итераций должен встретиться режим с углом")
}

func TestBuildPhotoSteering_OffImageAnglesGatedByFlags(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(7))
	for range 2000 {
		out := BuildPhotoSteering(r, false, false)
		assert.NotContains(t, out, photoCaptionAngle,
			"угол про подпись не должен предлагаться без caption")
		assert.NotContains(t, out, photoForwardAngle,
			"угол про форвард не должен предлагаться без пересылки")
	}
}

func TestPhotoAnglesNonSubstring(t *testing.T) {
	t.Parallel()
	angles := allPhotoAngles()
	for i, a := range angles {
		for j, b := range angles {
			if i == j {
				continue
			}
			require.NotContains(t, a, b, "угол %d не должен содержать угол %d как подстроку", i, j)
		}
	}
}
