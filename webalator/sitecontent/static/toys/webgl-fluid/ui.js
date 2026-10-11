'use strict';

let main = () => {
  let offscreen = document.querySelector('#fluid-viewer').transferControlToOffscreen();

  let worker = new Worker('./fluid.js');
  worker.postMessage({canvas: offscreen}, [offscreen])
};

document.addEventListener('DOMContentLoaded', (event) => main());
