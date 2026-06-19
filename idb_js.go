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
        const db = event.target.result;
        db.createObjectStore(STORE_NAME, { keyPath: 'name' });
      };
    });
  }

  async function getFile(name) {
    await init();
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], 'readonly');
      const store = transaction.objectStore(STORE_NAME);
      const request = store.get(name);
      request.onerror = (event) => {
        reject('Failed to retrieve file: ' + request.error);
      };
      request.onsuccess = (event) => {
        if (request.result) {
          resolve(request.result.content);
        } else {
          resolve(null); // Not found
        }
      };
    });
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
    deleteFile
  };
})();
`
