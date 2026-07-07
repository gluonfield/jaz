import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post } from './client'
import type { OnboardingInput, OnboardingState, OnboardingStatus } from './types'

export const onboardingStateQuery = queryOptions({
  queryKey: keys.onboardingState,
  queryFn: () => get<OnboardingState>('/v1/onboarding/state'),
})

export const onboardingQuery = queryOptions({
  queryKey: keys.onboarding,
  queryFn: () => get<OnboardingStatus>('/v1/onboarding'),
})

export function completeOnboarding(input: OnboardingInput): Promise<OnboardingStatus> {
  return post<OnboardingStatus>('/v1/onboarding', input)
}
