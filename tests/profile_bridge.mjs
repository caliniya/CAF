// Exercise the same ticketed bridge as the desktop host, with local component fixtures.
export async function loginTestProfile(apiURL) {
  const mock = window.go.Backend.Core
  window.go.Backend.Accounts = {
    StartVaultSession: async () => 'test-session',
    Call: async (ticket, name, args) => {
      if (ticket !== 'test-session') throw Error('inactive session')
      return (await mock[name](...JSON.parse(args))) ?? null
    },
  }
  await (await import(apiURL)).StartVaultSession()
}
