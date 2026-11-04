/**
 * The account, as the client needs it (02 Architecture/Backend Services.md).
 */

const KEY = 'universus.session'

export interface Account {
  id: number
  email: string
  displayName: string
}

interface Stored {
  token: string
  user: Account
}

function read(): Stored | null {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? (JSON.parse(raw) as Stored) : null
  } catch {
    // Private windows, cleared site data, storage disabled entirely.
    return null
  }
}

/**
 * The bearer token, or '' — which is exactly what an anonymous client sends.
 */
export function token(): string {
  return read()?.token ?? ''
}

export function account(): Account | null {
  return read()?.user ?? null
}

export function signOut(): void {
  try {
    localStorage.removeItem(KEY)
  } catch {
    // Nothing to remove if it could never be written.
  }
}

/** Registers or signs in, and remembers the session. */
export async function signIn(
  email: string,
  password: string,
  displayName?: string,
): Promise<Account> {
  const path = displayName === undefined ? '/api/auth/login' : '/api/auth/register'

  let res: Response
  try {
    res = await fetch(path, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(displayName === undefined ? {email, password} : {email, password, displayName}),
    })
  } catch {
    throw new Error('no server to sign in to')
  }

  // A server with no database does not register these routes at all, which
  // arrives here as a 404 and means something specific worth saying.
  if (res.status === 404) {
    throw new Error('this server has no accounts, so online play is not available')
  }

  const body = (await res.json().catch(() => ({}))) as {
    token?: string
    user?: Account
    error?: string
  }
  if (!res.ok || !body.token || !body.user) {
    throw new Error(body.error ?? `sign-in failed (${res.status})`)
  }

  try {
    localStorage.setItem(KEY, JSON.stringify({token: body.token, user: body.user}))
  } catch {
    // The session still works for this page; it just will not outlive it.
  }
  return body.user
}
