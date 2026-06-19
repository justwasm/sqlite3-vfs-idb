//go:build js && wasm

package idb

const idbJS = `
(async () => {
  const DB_NAME = 'sqlite_vfs_db';
  const STORE_NAME = 'files';
  let db;

  async function init() {
    if (db) return;
    return new Promise((resolve, reject) => {
      const request = indexedDB.open(DB_NAME, 1);
      request.onerror = (event) => {
        reject('IndexedDB error: ' + request.error);
      };
      request.onsuccess = (event) => {
        db = event.target.result;
        resolve(db);
      };
      request.onupgradeneeded = (event) => {
        event.target.result.createObjectStore(STORE_NAME, { keyPath: 'name' });
      };
    });
  }

  async function getFile(name) {
    await init();
    // When a previous WASM instance exits abnormally in a different
    // Worker, its pending readwrite transaction may not yet be aborted
    // by the browser. Retry with short delays so the new readonly
    // transaction sees committed data instead of a stale snapshot.
    var delay = 50;
    for (var attempt = 0; attempt < 4; attempt++) {
      var result = await new Promise(function (resolve, reject) {
        var tx = db.transaction([STORE_NAME], 'readonly');
        var store = tx.objectStore(STORE_NAME);
        var req = store.get(name);
        req.onerror = function () { reject('Failed to retrieve file: ' + req.error); };
        req.onsuccess = function () { resolve(req.result ? req.result.content : null); };
      });
      if (result !== null) return result;
      if (attempt < 3) await new Promise(function (r) { setTimeout(r, delay); });
      delay *= 2;
    }
    return null;
  }

  async function putFile(name, content) {
    await init();
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], 'readwrite');
      const store = transaction.objectStore(STORE_NAME);
      const request = store.put({ name: name, content: content });
      request.onerror = (event) => {
        reject('Failed to store file: ' + request.error);
      };
      request.onsuccess = (event) => {
        resolve();
      };
    });
  }

  async function deleteFile(name) {
    await init();
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], 'readwrite');
      const store = transaction.objectStore(STORE_NAME);
      const request = store.delete(name);
      request.onerror = (event) => {
        reject('Failed to delete file: ' + request.error);
      };
      request.onsuccess = (event) => {
        resolve();
      };
    });
  }

  globalThis.sqliteVFS = {
    getFile,
    putFile,
    deleteFile,
  };
})();
`
