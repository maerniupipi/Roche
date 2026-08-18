/**
 * Load a list exposed by an optional server feature.
 *
 * Disabled optional features may not register their API routes and therefore
 * return 404. Other transient failures should degrade the feature as well,
 * instead of blocking unrelated editor resources from loading.
 */
export async function loadOptionalFeatureList<T>(loader: () => Promise<T[]>): Promise<T[]> {
  try {
    const items = await loader()
    return Array.isArray(items) ? items : []
  } catch {
    return []
  }
}
