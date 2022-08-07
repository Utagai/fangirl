package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWrapWithRetry(t *testing.T) {
	testErr := errors.New("foo")
	errorsNever := func() error {
		return nil
	}

	errorsAlways := func() error {
		return testErr
	}

	makeNErrorFunc := func(maxErrs int) func() error {
		numErrs := 0
		return func() error {
			if numErrs < maxErrs {
				numErrs++
				return testErr
			}
			return nil
		}
	}

	// Set the testDelay to something tiny so we don't wait forever for these tests.
	// I think more ideally we'd use a mocked clock or whatever, but like... whatever.
	testDelay := 20 * time.Millisecond
	maxTries := 5

	// Sanity check the max tries we're testing against.
	if maxTries < 2 {
		t.Fatalf("maxTries should be set to a number greater than 2")
	}

	errorsOnce := makeNErrorFunc(1)
	errorsTwice := makeNErrorFunc(2)
	errorsMaxTimesExactly := makeNErrorFunc(maxTries)
	errorsOnceMoreThanMax := makeNErrorFunc(maxTries + 1)
	errorsOneLessThanMax := makeNErrorFunc(maxTries - 1)

	testCases := []struct {
		name        string
		fun         func() error
		expectedErr error
	}{
		{
			name:        "never erroring function",
			fun:         errorsNever,
			expectedErr: nil,
		},
		{
			name:        "always erroring function",
			fun:         errorsAlways,
			expectedErr: testErr,
		},
		{
			name:        "once erroring function",
			fun:         errorsOnce,
			expectedErr: nil,
		},
		{
			name:        "twice erroring function",
			fun:         errorsTwice,
			expectedErr: nil,
		},
		{
			name:        "ones less than max erroring function",
			fun:         errorsOneLessThanMax,
			expectedErr: nil,
		},
		{
			name:        "exactly max times erroring function",
			fun:         errorsMaxTimesExactly,
			expectedErr: nil,
		},
		{
			name:        "one less than max erroring function",
			fun:         errorsOnceMoreThanMax,
			expectedErr: testErr,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actualErr := wrapInRetry(tc.fun, uint(maxTries), testDelay)
			if tc.expectedErr != nil {
				assert.ErrorIs(t, actualErr, testErr)
			} else {
				assert.NoError(t, actualErr)
			}
		})
	}
}

func TestDelayIsHonored(t *testing.T) {
	maxTries := uint(2)
	retryDelay := 100 * time.Millisecond

	start := time.Now()
	err := wrapInRetry(func() error {
		return errors.New("blah")
	}, maxTries, retryDelay)
	end := time.Now()
	assert.Error(t, err)

	actualElapsed := end.Sub(start)

	// Since we have a maximum retry of 2, we would expect a delay after
	// the first failure, then another delay after the second failure,
	// and an immediate exit after the third. So, two delays:
	expectedElapsed := 2 * retryDelay

	// Now of course, there may be some delta here, since we can't have perfect timers:
	assert.InDelta(t, expectedElapsed.Milliseconds(), actualElapsed.Milliseconds(), 20)
}

func TestWrapInRetryWithRet(t *testing.T) {
	// Since wrapInRetryWithRet just wraps (no pun intended)
	// wrapWithRetry, we aren't going to try testing anything too crazy.

	testErr := errors.New("hi")
	numErrs := 0
	maxErrs := 2
	expectedRet := 42
	erroringFunc := func() (int, error) {
		if numErrs < maxErrs {
			numErrs++
			return 0, testErr
		}
		return expectedRet, nil
	}

	actualRet, err := wrapInRetryWithRet(erroringFunc, 5, 10*time.Millisecond)

	// We should not error.
	assert.NoError(t, err)

	// We should propagate the return value.
	assert.Equal(t, actualRet, expectedRet)

	// The function should have only been called maxErrs times.
	assert.Equal(t, numErrs, maxErrs)
}
