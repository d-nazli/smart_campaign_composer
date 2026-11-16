export interface Sample {
  /** Milliseconds (any clock, only differences matter). */
  at: number
  /** Total sent so far across all campaigns. */
  sent: number
}

/** Keeps the samples of the last `windowMs`, newest last. */
export function pushSample(samples: Sample[], s: Sample, windowMs = 10_000): Sample[] {
  return [...samples, s].filter((x) => s.at - x.at <= windowMs)
}

/** Messages per second over the sampled window. 0 with fewer than two samples or when the total went down. */
export function ratePerSecond(samples: Sample[]): number {
  if (samples.length < 2) return 0
  const first = samples[0]
  const last = samples[samples.length - 1]
  const seconds = (last.at - first.at) / 1000
  const sent = last.sent - first.sent
  if (seconds <= 0 || sent < 0) return 0
  return sent / seconds
}
