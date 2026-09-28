import { useEffect, useState } from 'react';

export interface ApiState<T> {
  data: T | null;
  loading: boolean;
  error: Error | null;
}

export function useApi<T>(fetcher: () => Promise<T>, deps: any[] = []): ApiState<T> {
  const [state, setState] = useState<ApiState<T>>({
    data: null,
    loading: true,
    error: null,
  });

  useEffect(() => {
    let cancelled = false;

    setState((prev) => ({ ...prev, loading: true, error: null }));

    fetcher()
      .then((data) => {
        if (!cancelled) {
          setState({ data, loading: false, error: null });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setState({ data: null, loading: false, error });
        }
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return state;
}
