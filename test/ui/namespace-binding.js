async function discoveryMode(page) {
  await page.waitForFunction(() => {
    const session = document.querySelectorAll('.context-meta code')[2]?.textContent || '';
    return session && session !== 'NOT_BOUND';
  }, undefined, { timeout: 120000 });
  await page.locator('.context-meta code').nth(2).waitFor({ state: 'visible' });
  const sessionID = await page.locator('.context-meta code').nth(2).innerText();
  const discovery = await page.evaluate(async session => {
    const response = await fetch(`/api/v09/environments/${encodeURIComponent(session)}/namespaces`, { headers: { Accept: 'application/json' } });
    return { status: response.status, body: await response.json() };
  }, sessionID);
  if (discovery.status !== 200) throw new Error(`namespace discovery returned ${discovery.status} for session ${sessionID}: ${JSON.stringify(discovery.body)}`);
  const mode = discovery.body.mode || discovery.body.Mode;
  if (mode !== 'DISCOVERED' && mode !== 'EXPLICIT_ONLY') throw new Error(`unsupported namespace discovery mode: ${mode}`);
  return { mode, sessionID };
}

async function openContextControls(page) {
  const control = page.getByTestId('global-context');
  const identity = page.getByTestId('context-identity');
  if (await control.count() && !(await identity.isVisible())) await control.locator('summary').click();
}

async function closeContextControls(page) {
  const control = page.getByTestId('global-context');
  const identity = page.getByTestId('context-identity');
  if (await control.count() && await identity.isVisible()) await control.locator('summary').click();
}

async function waitForBinding(page, namespace) {
  await page.waitForFunction(expected => {
    const meta = document.querySelector('.context-meta')?.textContent || '';
    return meta.includes(`Namespace ${expected}`) && /Version\s+\d+/.test(meta);
  }, namespace, { timeout: 120000 });
  const codes = await page.locator('.context-meta code').allTextContents();
  const boundNamespace = codes[1];
  const sessionID = codes[2];
  const contextVersion = codes[3];
  if (boundNamespace !== namespace) throw new Error(`authoritative namespace binding mismatch: requested=${namespace} bound=${boundNamespace}`);
  if (!sessionID || sessionID === 'NOT_BOUND' || !/^\d+$/.test(contextVersion)) throw new Error('authoritative environment session binding did not complete');
  return { namespace: boundNamespace, sessionID, contextVersion };
}

async function bindNamespace(page, namespace, identity) {
  await openContextControls(page);
  await page.getByTestId('context-identity').locator('option').nth(1).waitFor({ state: 'attached' });
  await page.getByTestId('context-identity').selectOption(identity);
  const { mode, sessionID } = await discoveryMode(page);
  if (mode === 'DISCOVERED') {
    try {
      await page.waitForFunction(expected => {
        const meta = document.querySelector('.context-meta')?.textContent || '';
        return meta.includes(`Namespace ${expected}`) && /Version\s+\d+/.test(meta);
      }, namespace, { timeout: 5000 });
      const binding = await waitForBinding(page, namespace);
      await closeContextControls(page);
      return { mode, requestedNamespace: namespace, sessionID, ...binding };
    } catch {
      // The identity's default binding is not the requested namespace; select it explicitly.
    }
    const select = page.getByTestId('context-namespace');
    await select.locator(`option[value="${namespace}"]`).waitFor({ state: 'attached' });
    const response = page.waitForResponse(item => item.url().includes('/capabilities?') && item.url().includes(`namespace=${encodeURIComponent(namespace)}`) && item.request().method() === 'GET');
    await select.selectOption(namespace);
    const boundResponse = await response;
    if (boundResponse.status() !== 200) throw new Error(`namespace binding returned ${boundResponse.status()}`);
  } else {
    const response = page.waitForResponse(item => item.url().includes('/capabilities?') && item.url().includes(`namespace=${encodeURIComponent(namespace)}`) && item.request().method() === 'GET');
    const input = page.getByTestId('explicit-namespace');
    await input.fill(namespace);
    await page.getByTestId('open-explicit-namespace').click();
    const boundResponse = await response;
    if (boundResponse.status() !== 200) throw new Error(`namespace binding returned ${boundResponse.status()}`);
  }
  const binding = await waitForBinding(page, namespace);
  await closeContextControls(page);
  return { mode, requestedNamespace: namespace, sessionID, ...binding };
}

module.exports = { bindNamespace, waitForBinding, openContextControls };
