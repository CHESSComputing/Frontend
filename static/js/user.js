/*
 * JS functions to handle user info
 * Author: Valentin Kuznetsov, 2026
 */

// Below is JS code to handle user info overlay
document.addEventListener('click', function(e) {
  const btn = e.target.closest('.user-info-btn');
  if (!btn) return;
  e.preventDefault();

  const url = btn.getAttribute('href');
  const overlay = document.getElementById('user-popup-overlay');
  const content = document.getElementById('user-popup-content');
  const header  = document.getElementById('user-popup-header');

  content.innerHTML = '<p style="color:#888">&nbsp;Loading...</p>';
  header.classList.remove('visible');
  overlay.classList.add('active');

  fetch(url, { headers: { 'Accept': 'application/json' } })
    .then(r => r.json())
    .then(data => {
      content.innerHTML = renderUser(data);
      header.classList.add('visible');
    })
    .catch(() => { content.innerHTML = '<p>ERROR: failed to load user information</p>'; });
});

document.getElementById('user-popup-close')
  .addEventListener('click', closeUserPopup);

document.getElementById('user-popup-overlay')
  .addEventListener('click', function(e) {
    if (e.target === this) closeUserPopup();
  });

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') closeUserPopup();
});

function closeUserPopup() {
  const overlay = document.getElementById('user-popup-overlay');
  const header  = document.getElementById('user-popup-header');
  const content = document.getElementById('user-popup-content');
  overlay.classList.remove('active');
  header.classList.remove('visible');
  content.innerHTML = '';
}

function renderUser(data) {
  const users = Array.isArray(data) ? data : [data];

  if (users.length === 0)
    return '<p style="color:#888">No records found.</p>';

  const header = users.length > 1
    ? `<p style="font-size:12px;color:#888;margin:0 0 12px">
         ${users.length} records found
       </p>`
    : '';

  return header + users.map((u, i) => renderRecord(u, i, users.length)).join('');
}

function renderRecord(u, index, total) {
  const row = (label, val) => val
    ? `<tr>
         <td style="color:#888;padding:4px 12px 4px 0;
                    white-space:nowrap;font-size:13px">${label}</td>
         <td style="padding:4px 0;font-size:13px">${val}</td>
       </tr>` : '';

  const chips = (items) => (items || [])
    .map(i => `<span style="display:inline-block;
      background:#f1f1f1;border-radius:4px;
      padding:2px 8px;margin:2px;font-size:12px">${i}</span>`)
    .join('');

  const divider = index > 0
    ? `<hr style="border:none;border-top:0.5px solid #e0e0e0;margin:16px 0">`
    : '';

  const badge = total > 1
    ? `<span style="font-size:11px;color:#888;
                   float:right;margin-top:3px">
         ${index + 1} / ${total}
       </span>` : '';

  return `
    ${divider}
    ${badge}
    <h3 style="margin:0 0 2px;font-size:16px;
               font-weight:500">&#128100; ${u.Name}</h3>
    <p style="margin:0 0 12px;color:#888;
              font-size:13px">&#128231; ${u.Email}</p>
    <table style="width:100%;border-collapse:collapse">
      ${row('UID', u.Uid)}
      ${row('UID number', u.UidNumber)}
      ${row('GID number', u.GidNumber)}
      ${row('Expires', u.Expire?.slice(0, 10))}
    </table>
    ${u.Beamlines?.length ? `
      <p style="margin:10px 0 4px;font-size:11px;
                color:#888;letter-spacing:.05em">BEAMLINES</p>
      ${chips(u.Beamlines)}` : ''}
    ${u.Btrs?.length ? `
      <p style="margin:10px 0 4px;font-size:11px;
                color:#888;letter-spacing:.05em">BTRs</p>
      ${chips(u.Btrs)}` : ''}
    ${u.Foxdens?.length ? `
      <p style="margin:10px 0 4px;font-size:11px;
                color:#888;letter-spacing:.05em">FOXDENS</p>
      ${chips(u.Foxdens)}` : ''}`;
}
