'use strict';

let main = () => {
  let offscreen = document.querySelector('#viewer').transferControlToOffscreen()
  let worker = new Worker('ants.js');
  worker.postMessage({canvas: offscreen}, [offscreen]);
};

document.addEventListener('DOMContentLoaded', (event) => main());
