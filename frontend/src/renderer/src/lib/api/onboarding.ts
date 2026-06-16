import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post } from './client'
import type { OnboardingInput, OnboardingStatus } from './types'

export const onboardingQuery = queryOptions({
  queryKey: keys.onboarding,
  queryFn: () => get<OnboardingStatus>('/v1/onboarding'),
})

export function completeOnboarding(input: OnboardingInput): Promise<OnboardingStatus> {
  return post<OnboardingStatus>('/v1/onboarding', input)
}
