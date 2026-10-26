package linkcontext

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/reijo1337/ToxicBot/internal/infrastructure/linkpreview"
	pkglogger "github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec: LINK-023
// spec: PHOTO-026
// spec: PHOTO-027
func TestVisionSharesPhotoAndPreviewConcurrencyAndRate(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		limits := NewLimits()
		var calls, running atomic.Int64
		finishPhoto := make(chan struct{})
		finishPreviews := make(chan struct{})
		photoCtx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		photoDeadline, _ := photoCtx.Deadline()
		client := visionFunc(func(ctx context.Context, prompt string, data []byte) (string, error) {
			calls.Add(1)
			running.Add(1)
			defer running.Add(-1)
			finish := finishPreviews
			if string(data) == "ordinary photo" {
				assert.Equal(t, "unchanged ordinary photo prompt", prompt)
				assert.Equal(t, photoCtx, ctx)
				deadline, ok := ctx.Deadline()
				assert.True(t, ok)
				assert.Equal(t, photoDeadline, deadline)
				finish = finishPhoto
			}
			select {
			case <-finish:
				return "description", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
		photo := NewVision(client, limits)
		photoDone := make(chan error, 1)
		go func() {
			_, err := photo.GenerateContent(
				photoCtx,
				"unchanged ordinary photo prompt",
				[]byte("ordinary photo"),
			)
			photoDone <- err
		}()
		synctest.Wait()
		require.EqualValues(t, 1, running.Load())

		enricher := New(t.Context(), loaderStub{
			page: func(context.Context, string) (linkpreview.Metadata, error) {
				return linkpreview.Metadata{Title: "title", ImageURL: "https://img.example/p"}, nil
			},
			image: func(context.Context, string) ([]byte, error) { return []byte("preview"), nil },
		}, client, pkglogger.New(), limits)
		defer enricher.Close()
		for _, rawURL := range []string{"https://example.com/one", "https://example.com/two"} {
			go enricher.Enrich(t.Context(), 1, Reference{URL: rawURL})
		}
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load())
		require.EqualValues(t, 2, running.Load())

		time.Sleep(6 * time.Second)
		require.EqualValues(t, 2, calls.Load())
		close(finishPhoto)
		require.NoError(t, <-photoDone)
		synctest.Wait()
		require.EqualValues(t, 3, calls.Load())
		require.EqualValues(t, 2, running.Load())
		close(finishPreviews)
		synctest.Wait()
		require.Zero(t, running.Load())

		before := time.Now()
		text, err := photo.GenerateContent(
			photoCtx,
			"another ordinary photo",
			[]byte("ordinary photo"),
		)
		require.Error(t, err)
		require.Empty(t, text)
		require.Equal(t, time.Duration(0), time.Since(before))
		require.EqualValues(t, 3, calls.Load())
	})
}

// spec: LINK-023
// spec: PHOTO-027
func TestOrdinaryPhotoRejectsBusyVisionWithoutQueue(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		finish := make(chan struct{})
		defer close(finish)
		photo := NewVision(
			visionFunc(func(ctx context.Context, _ string, _ []byte) (string, error) {
				calls.Add(1)
				select {
				case <-finish:
					return "description", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}),
			NewLimits(),
		)
		for range 2 {
			go func() { _, _ = photo.GenerateContent(t.Context(), "photo", []byte("image")) }()
		}
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load())
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		started := time.Now()
		text, err := photo.GenerateContent(ctx, "third photo", []byte("image"))
		require.Error(t, err)
		require.Empty(t, text)
		require.Equal(t, time.Duration(0), time.Since(started))
		require.EqualValues(t, 2, calls.Load())
	})
}

// spec: LINK-023
// spec: PHOTO-026
func TestOrdinaryPhotoSharesPreviewQuotaAndRefill(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const sharedDescription = "shared vision description"
		limits := NewLimits()
		var calls atomic.Int64
		client := visionFunc(func(context.Context, string, []byte) (string, error) {
			calls.Add(1)
			return sharedDescription, nil
		})
		preview := newPreviewVision(client, limits)
		photo := NewVision(client, limits)
		for range 2 {
			text, err := preview.GenerateContent(t.Context(), "preview", []byte("preview"))
			require.NoError(t, err)
			require.Equal(t, sharedDescription, text)
		}
		before := time.Now()
		text, err := photo.GenerateContent(t.Context(), "photo", []byte("photo"))
		require.ErrorIs(t, err, errVisionLimited)
		require.Empty(t, text)
		require.Equal(t, time.Duration(0), time.Since(before))
		require.EqualValues(t, 2, calls.Load())

		time.Sleep(6 * time.Second)
		text, err = photo.GenerateContent(t.Context(), "photo", []byte("photo"))
		require.NoError(t, err)
		require.Equal(t, sharedDescription, text)
		require.EqualValues(t, 3, calls.Load())
		_, err = preview.GenerateContent(t.Context(), "preview", []byte("preview"))
		require.ErrorIs(t, err, errVisionLimited)
		require.EqualValues(t, 3, calls.Load())
	})
}
