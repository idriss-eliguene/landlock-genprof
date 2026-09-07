package main

import "net/http"

func handleWorkbenchScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/workbench.js" {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write([]byte(workbenchScript))
}

const workbenchScript = `(function () {
  "use strict";
  const app = document.getElementById("observation-workbench");
  if (!app) return;
  const picker = app.querySelector("#workload-picker"), detail = app.querySelector("#observation-detail"), message = app.querySelector("#workbench-message");
  const esc = (v) => String(v == null ? "" : v).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
  const state = (v) => '<span class="state ' + (v === "UNKNOWN" ? "unknown" : "") + '">' + esc(v || "NOT_AVAILABLE") + '</span>';
  function showError(e) { message.textContent = e && e.reason ? e.reason : "Read failed; authoritative state was not changed."; }
  async function get(url, options) { const r = await fetch(url, Object.assign({headers:{"Accept":"application/json"}}, options || {})); const b = await r.json().catch(() => ({})); if (!r.ok) throw b; return b; }
  function selector(c) { return new URLSearchParams({kind:c.kind,name:c.name,container:c.container,workloadUID:c.uid,group:c.group || "",imageIdentity:c.image || ""}); }
  function renderObservation(o) { detail.innerHTML = '<h3>Observation <code>' + esc(o.observationID) + '</code></h3><p><b>Execution:</b> ' + state(o.execution.state) + ' <b>Frozen:</b> ' + esc(o.frozen) + '</p><p><b>Observed workload identity:</b> ' + esc(o.identity.namespace + " / " + o.identity.group + "/" + o.identity.kind + "/" + o.identity.workloadName) + ' · UID <code>' + esc(o.identity.workloadUID) + '</code> · ' + esc(o.identity.container) + '</p><table><thead><tr><th>Source</th><th>Attribution</th><th>Evidence</th><th>Facts</th></tr></thead><tbody>' + (o.sources || []).map(s => '<tr><td>' + esc(s.name) + '</td><td>' + state(s.attributionState) + '</td><td>' + state(s.evidenceState) + '</td><td>' + esc(s.attributedCount) + ' attributed; ' + esc(s.excludedCount) + ' excluded<br><code>' + esc(JSON.stringify(s.facts || {})) + '</code></td></tr>').join("") + '</tbody></table>'; }
  async function load(c) { const q = selector(c); try { const o = await get('/api/observations?' + q); app.querySelector('#observation-list').innerHTML = (o.items || []).map(x => '<li><button data-id="' + esc(x.observationID) + '">' + esc(x.observationID) + '</button> ' + state(x.execution.state) + '</li>').join("") || '<li>No durable Observations for this identity.</li>'; app.querySelectorAll('#observation-list button').forEach(b => b.onclick = async () => renderObservation(await get('/api/observations/' + encodeURIComponent(b.dataset.id)))); const p = await get('/api/proposals?' + q); app.querySelector('#proposal-list').innerHTML = (p.items || []).map(x => '<li><a href="/api/proposals/' + encodeURIComponent(x.name) + '">' + esc(x.name) + '</a> ' + state(x.status.approvalState) + '</li>').join("") || '<li>No durable Proposals for this subject.</li>'; } catch(e) { showError(e); } }
  async function workloads() { try { picker.length=1; const d = await get('/api/workloads'); (d.workloads || []).forEach(w => (w.pods || []).forEach(p => (p.containers || []).forEach(c => { if (!c.target) return; const o = document.createElement('option'); o.textContent = w.target.kind + '/' + w.target.name + ' · ' + c.name + ' · pod ' + p.uid; o.dataset.context = JSON.stringify({pod:p.name,group:w.target.group,kind:w.target.kind,name:w.target.name,container:c.name,uid:w.uid || p.uid,image:c.runtime && c.runtime.imageID || ""}); picker.appendChild(o); }))); } catch(e) { showError(e); } }
  let selectedContext = null, selectedObservation = null;
  async function post(url, body) { return get(url, {method:"POST", headers:{"Accept":"application/json","Content-Type":"application/json"}, body:JSON.stringify(body)}); }
  picker.onchange = () => { if (picker.selectedIndex) { selectedContext = JSON.parse(picker.options[picker.selectedIndex].dataset.context); load(selectedContext); } };
  app.querySelector('#observation-list').addEventListener('click', async (e) => { if (e.target.tagName !== 'BUTTON') return; try { selectedObservation = await get('/api/observations/' + encodeURIComponent(e.target.dataset.id)); renderObservation(selectedObservation); } catch (x) { showError(x); } });
  document.querySelector('#start-observation').onclick = async () => { if (!selectedContext) return showError({reason:"Select a workload/container first."}); try { await post('/api/observations/start', {namespace:"{{.Namespace}}",pod:selectedContext.pod,container:selectedContext.container,sources:["filesystem"],duration:60000000000}); await load(selectedContext); } catch (x) { showError(x); } };
  document.querySelector('#stop-observation').onclick = async () => { if (!selectedObservation) return showError({reason:"Select an Observation first."}); try { await post('/api/observations/stop', {namespace:"{{.Namespace}}",observationID:selectedObservation.observationID}); await load(selectedContext); } catch (x) { showError(x); } };
  document.querySelector('#generate-proposal').onclick = async () => { if (!selectedObservation) return showError({reason:"Select a completed Observation first."}); try { await post('/api/observations/generate-proposal', {namespace:"{{.Namespace}}",observationID:selectedObservation.observationID,proposalName:"observation-" + selectedObservation.observationID}); await load(selectedContext); } catch (x) { showError(x); } };
  app.querySelector('#refresh-workbench').onclick = workloads; workloads();
})();`
