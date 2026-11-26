'use strict';

// Folder uploader for the upload page.  The user picks (or drops) a folder,
// and each image or video in it is sent to the server as a separate request,
// named after its path within the folder.
//
// This uses React without a build step, so there is no JSX: elements are made
// with React.createElement directly.

(function() {

const e = React.createElement;

// How many files are uploaded at once.
const concurrency = 3;

// mediaExtensions are the files worth sending.  The server makes the final
// decision (and reports anything else as skipped), but filtering here avoids
// uploading a folder's worth of sidecar files just to have them rejected.
const mediaExtensions = new Set([
  'avif', 'bmp', 'gif', 'heic', 'heif', 'jpeg', 'jpg', 'png', 'svg', 'tif',
  'tiff', 'webp', 'm4v', 'mov', 'mp4', 'webm',
]);

// isMediaPath reports whether a file at path (relative to the picked folder)
// should be uploaded: it looks like an image or video, and isn't hidden or
// macOS metadata.
function isMediaPath(path) {
  const segments = path.split('/');
  if (segments.some(s => s.startsWith('.') || s === '__MACOSX')) {
    return false;
  }
  const base = segments[segments.length - 1];
  const dot = base.lastIndexOf('.');
  return dot >= 0 && mediaExtensions.has(base.slice(dot + 1).toLowerCase());
}

function humanBytes(n) {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return (i === 0 ? n.toString() : n.toFixed(1)) + ' ' + units[i];
}

// readAllEntries reads every entry of a directory.  readEntries returns them
// in batches, and an empty batch means the end.
function readAllEntries(reader) {
  return new Promise((resolve, reject) => {
    const all = [];
    const next = () => {
      reader.readEntries(batch => {
        if (batch.length === 0) {
          resolve(all);
          return;
        }
        all.push(...batch);
        next();
      }, reject);
    };
    next();
  });
}

// walkEntry collects the files under a dropped file or directory entry into
// out, as {path, file}.
async function walkEntry(entry, out) {
  if (entry.isFile) {
    const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
    out.push({path: entry.fullPath.replace(/^\/+/, ''), file: file});
  } else if (entry.isDirectory) {
    for (const child of await readAllEntries(entry.createReader())) {
      await walkEntry(child, out);
    }
  }
}

// filesFromDrop collects the files in a drop, descending into folders.
async function filesFromDrop(dataTransfer) {
  // Entries must be taken from the items synchronously, before the first
  // await, or the browser clears them.
  const entries = [];
  for (const item of dataTransfer.items) {
    const entry = item.webkitGetAsEntry ? item.webkitGetAsEntry() : null;
    if (entry) {
      entries.push(entry);
    }
  }
  const out = [];
  for (const entry of entries) {
    await walkEntry(entry, out);
  }
  return out;
}

// filesFromInput collects the files from a directory picker.
function filesFromInput(input) {
  return Array.from(input.files).map(file => ({
    path: file.webkitRelativePath || file.name,
    file: file,
  }));
}

// uploadFile sends one file, calling onProgress with the bytes sent so far.
// It resolves to the server's JSON response, or {error} on failure.
function uploadFile(endpoint, folder, item, onProgress) {
  const url = new URL(endpoint, window.location.href);
  url.searchParams.set('folder', folder);
  url.searchParams.set('path', item.path);

  return new Promise(resolve => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url.toString());
    xhr.setRequestHeader('Content-Type', item.file.type || 'application/octet-stream');
    xhr.upload.onprogress = ev => onProgress(ev.loaded);
    xhr.onload = () => {
      let body = null;
      try {
        body = JSON.parse(xhr.responseText);
      } catch (err) {
        // Not JSON; perhaps an error page from a proxy.
      }
      if (xhr.status === 200 && body && body.outcome) {
        resolve(body);
      } else if (body && body.error) {
        resolve({error: body.error});
      } else {
        resolve({error: `The server responded ${xhr.status} ${xhr.statusText}`});
      }
    };
    xhr.onerror = () => resolve({error: 'Network error'});
    xhr.ontimeout = () => resolve({error: 'Timed out'});
    xhr.send(item.file);
  });
}

// itemsReducer manages the list of files being uploaded.  Each item is
// {id, path, file, size, status, loaded, name, error}, where status is one of
// pending, uploading, stored, existed, skipped, or failed.
function itemsReducer(items, action) {
  switch (action.type) {
  case 'replace':
    return action.items;
  case 'update':
    return items.map(it => it.id === action.id ? Object.assign({}, it, action.patch) : it);
  case 'retry':
    return items.map(it => (it.status === 'failed' && !it.tooLarge) ?
                     Object.assign({}, it, {status: 'pending', loaded: 0, error: ''}) : it);
  default:
    throw new Error('unknown action ' + action.type);
  }
}

let ProgressBar = ({fraction, variant, label}) => {
  const pct = Math.round(Math.min(1, Math.max(0, fraction)) * 100);
  return e('div', {className: 'progress', role: 'progressbar', 'aria-label': label,
                   'aria-valuenow': pct, 'aria-valuemin': 0, 'aria-valuemax': 100},
           e('div', {className: 'progress-bar' + (variant ? ' bg-' + variant : ''),
                     style: {width: pct + '%'}}));
};

let StatusCell = ({item, viewLink}) => {
  switch (item.status) {
  case 'pending':
    return e('span', {className: 'badge text-bg-secondary'}, 'Waiting');
  case 'uploading':
    return e(ProgressBar, {fraction: item.size ? item.loaded / item.size : 0,
                           label: 'Uploading ' + item.path});
  case 'stored':
    return e('span', {},
             e('span', {className: 'badge text-bg-success me-2'}, 'Uploaded'),
             e('a', {href: viewLink(item.name), className: 'small'}, 'view'));
  case 'existed':
    return e('span', {},
             e('span', {className: 'badge text-bg-info me-2'}, 'Already there'),
             e('a', {href: viewLink(item.name), className: 'small'}, 'view'));
  case 'skipped':
    return e('span', {className: 'badge text-bg-warning'}, 'Skipped: not an image or video');
  case 'failed':
    return e('span', {className: 'text-danger small'}, item.error);
  }
  return null;
};

let FileRow = React.memo(({item, viewLink}) =>
  e('tr', {},
    e('td', {className: 'text-break'}, item.path),
    e('td', {className: 'text-nowrap text-end'}, humanBytes(item.size)),
    e('td', {style: {width: '30%'}}, e(StatusCell, {item: item, viewLink: viewLink}))));

let FileTable = ({items, viewLink}) =>
  e('table', {className: 'table table-sm align-middle'},
    e('thead', {},
      e('tr', {},
        e('th', {scope: 'col'}, 'File'),
        e('th', {scope: 'col', className: 'text-end'}, 'Size'),
        e('th', {scope: 'col'}, 'Status'))),
    e('tbody', {},
      items.map(it => e(FileRow, {key: it.id, item: it, viewLink: viewLink}))));

let Summary = ({items, running, folder}) => {
  const counts = {pending: 0, uploading: 0, stored: 0, existed: 0, skipped: 0, failed: 0};
  let totalBytes = 0;
  let doneBytes = 0;
  for (const it of items) {
    counts[it.status]++;
    if (it.tooLarge) {
      // Never sent, so it shouldn't make the progress bar look further
      // along.
      continue;
    }
    totalBytes += it.size;
    if (it.status === 'uploading') {
      doneBytes += it.loaded;
    } else if (it.status !== 'pending') {
      doneBytes += it.size;
    }
  }
  const finished = items.length - counts.pending - counts.uploading;

  const parts = [];
  if (counts.stored) parts.push(`${counts.stored} uploaded`);
  if (counts.existed) parts.push(`${counts.existed} already there`);
  if (counts.skipped) parts.push(`${counts.skipped} skipped`);
  if (counts.failed) parts.push(`${counts.failed} failed`);

  return e('div', {className: 'mb-3'},
           e('p', {className: 'mb-1'},
             running ? `Uploading into ${folder ? '"' + folder + '"' : 'the top of the collection'}: ` : '',
             `${finished} of ${items.length} files done (${humanBytes(doneBytes)} of ${humanBytes(totalBytes)})`,
             parts.length ? ' — ' + parts.join(', ') : ''),
           e(ProgressBar, {fraction: totalBytes ? doneBytes / totalBytes : (finished / Math.max(1, items.length)),
                           variant: counts.failed ? 'warning' : (running ? '' : 'success'),
                           label: 'Overall progress'}));
};

let DropZone = ({disabled, onFiles}) => {
  const [hover, setHover] = React.useState(false);
  const [scanning, setScanning] = React.useState(false);
  const inputRef = React.useRef(null);

  const onDrop = async (ev) => {
    ev.preventDefault();
    setHover(false);
    if (disabled) {
      return;
    }
    setScanning(true);
    try {
      onFiles(await filesFromDrop(ev.dataTransfer));
    } finally {
      setScanning(false);
    }
  };

  const onChange = () => {
    const input = inputRef.current;
    onFiles(filesFromInput(input));
    // Allow picking the same folder again (for example, to retry).
    input.value = '';
  };

  return e('div', {
             className: 'mb-3 p-5 border border-2 rounded text-center text-body-secondary' + (hover ? ' bg-body-tertiary' : ''),
             style: {borderStyle: 'dashed'},
             onDragOver: ev => { ev.preventDefault(); setHover(true); },
             onDragLeave: () => setHover(false),
             onDrop: onDrop,
           },
           e('p', {className: 'mb-2'},
             scanning ? 'Reading folder…' : 'Drop a folder here, or choose one.'),
           e('p', {className: 'mb-2 small'},
             'Each image or video in the folder is uploaded under its path within the folder, inside the folder above.  ' +
             'Files that already exist are left unchanged, so an interrupted upload can be retried by choosing the folder again.'),
           e('input', {ref: inputRef, type: 'file', id: 'folder-input', className: 'd-none',
                       webkitdirectory: '', multiple: true, disabled: disabled, onChange: onChange}),
           e('label', {htmlFor: 'folder-input',
                       className: 'btn btn-outline-primary' + (disabled ? ' disabled' : '')},
             'Choose folder…'));
};

let FolderUploader = ({endpoint, collectionID, maxBytes, folderInput}) => {
  const [items, dispatch] = React.useReducer(itemsReducer, []);
  const [ignored, setIgnored] = React.useState([]);
  const [running, setRunning] = React.useState(false);
  const [folder, setFolder] = React.useState('');

  // The latest items, for the upload loop, which outlives any one render.
  const itemsRef = React.useRef(items);
  itemsRef.current = items;

  React.useEffect(() => {
    if (!running) {
      return undefined;
    }
    const warn = ev => { ev.preventDefault(); ev.returnValue = ''; };
    window.addEventListener('beforeunload', warn);
    if (folderInput) folderInput.readOnly = true;
    return () => {
      window.removeEventListener('beforeunload', warn);
      if (folderInput) folderInput.readOnly = false;
    };
  }, [running]);

  const viewLink = React.useCallback(name => {
    const q = new URLSearchParams({collection: collectionID, name: name});
    return '/view-object?' + q.toString();
  }, [collectionID]);

  const onFiles = found => {
    const media = [];
    const skipped = [];
    for (const f of found) {
      (isMediaPath(f.path) ? media : skipped).push(f.path);
    }
    media.sort();
    const byPath = new Map(found.map(f => [f.path, f.file]));
    dispatch({type: 'replace', items: media.map((path, i) => {
      const file = byPath.get(path);
      const tooLarge = file.size > maxBytes;
      return {
        id: i,
        path: path,
        file: file,
        size: file.size,
        status: tooLarge ? 'failed' : 'pending',
        tooLarge: tooLarge,
        error: tooLarge ? `Too large; files can be at most ${humanBytes(maxBytes)}` : '',
        loaded: 0,
        name: '',
      };
    })});
    setIgnored(skipped.sort());
  };

  // run uploads the items in queue.
  const run = async (queue) => {
    const target = folderInput ? folderInput.value : '';
    setFolder(target.trim());
    setRunning(true);

    queue = queue.slice();
    const worker = async () => {
      while (queue.length > 0) {
        const item = queue.shift();
        dispatch({type: 'update', id: item.id, patch: {status: 'uploading', loaded: 0}});
        const resp = await uploadFile(endpoint, target, item, loaded =>
          dispatch({type: 'update', id: item.id, patch: {loaded: loaded}}));
        if (resp.error) {
          dispatch({type: 'update', id: item.id, patch: {status: 'failed', error: resp.error}});
        } else {
          dispatch({type: 'update', id: item.id, patch: {status: resp.outcome, name: resp.name}});
        }
      }
    };
    const workers = [];
    for (let i = 0; i < concurrency; i++) {
      workers.push(worker());
    }
    await Promise.all(workers);
    setRunning(false);
  };

  const start = () => run(itemsRef.current.filter(it => it.status === 'pending'));

  const retry = () => {
    const queue = itemsRef.current.filter(it => it.status === 'failed' && !it.tooLarge);
    dispatch({type: 'retry'});
    run(queue);
  };

  const pending = items.filter(it => it.status === 'pending').length;
  const retryable = items.filter(it => it.status === 'failed' && !it.tooLarge).length;

  const buttons = [];
  if (!running && pending > 0) {
    buttons.push(e('button', {key: 'start', type: 'button', className: 'btn btn-primary me-2', onClick: start},
                   `Upload ${pending} file${pending === 1 ? '' : 's'}`));
  }
  if (!running && retryable > 0) {
    buttons.push(e('button', {key: 'retry', type: 'button', className: 'btn btn-warning me-2', onClick: retry},
                   `Retry ${retryable} failed`));
  }
  if (!running && items.length > 0) {
    buttons.push(e('button', {key: 'clear', type: 'button', className: 'btn btn-outline-secondary me-2',
                              onClick: () => { dispatch({type: 'replace', items: []}); setIgnored([]); }},
                   'Clear'));
  }

  return e(React.Fragment, {},
           running ? null : e(DropZone, {disabled: running, onFiles: onFiles}),
           items.length > 0 ? e(Summary, {items: items, running: running, folder: folder}) : null,
           buttons.length > 0 ? e('div', {className: 'mb-3'}, buttons) : null,
           ignored.length > 0 ?
             e('details', {className: 'mb-3 small text-body-secondary'},
               e('summary', {}, `${ignored.length} other file${ignored.length === 1 ? '' : 's'} ignored (not images or videos, or hidden)`),
               e('ul', {}, ignored.map(p => e('li', {key: p, className: 'text-break'}, p)))) : null,
           items.length > 0 ? e(FileTable, {items: items, viewLink: viewLink}) : null);
};

const container = document.getElementById('folder-uploader');
if (container) {
  ReactDOM.createRoot(container).render(e(FolderUploader, {
    endpoint: container.dataset.endpoint,
    collectionID: container.dataset.collection,
    maxBytes: parseInt(container.dataset.maxBytes, 10),
    folderInput: document.getElementById('folder'),
  }));
}

})();
