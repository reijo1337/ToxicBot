package message

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPhotoSteering(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))

	var sawFree, sawAngle bool
	for range 2000 {
		out := BuildPhotoSteering(r)

		require.Contains(t, out, photoSteeringAntiTemplate,
			"в каждой подсказке должно быть анти-шаблонное правило")

		angleCount := 0
		for _, a := range photoAngles {
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

func TestPhotoAnglesNonSubstring(t *testing.T) {
	t.Parallel()

	for i, a := range photoAngles {
		for j, b := range photoAngles {
			if i == j {
				continue
			}
			require.NotContains(t, a, b,
				"угол %d не должен содержать угол %d как подстроку", i, j)
		}
	}
}
