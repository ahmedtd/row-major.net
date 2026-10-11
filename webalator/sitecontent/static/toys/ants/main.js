'use strict';

let main = () => {
  let viewer = document.querySelector('#viewer');
  let offscreen = viewer.transferControlToOffscreen();
  let worker = new Worker('ants.js');
  worker.postMessage({canvas: offscreen}, [offscreen]);

  viewer.addEventListener
};

document.addEventListener('DOMContentLoaded', (event) => main());

// AntSimulationHost is the host side of the ant simulation logic.
class AntSimulationHost {
  canvasElement = null;
  worker = null;

  constructor(canvasElement) {
	this.canvasElement = canvasElement;
	this.worker = null;
  }

  start() {
	let offscreenCanvas = this.canvasElement.transferControlToOffscreen();
	this.worker = new Worker('ants.js');
	worker.postMessage({type: 'init', canvas: offscreenCanvas}, [offscreenCanvas]);

	this.canvasElement.addEventListener('mousedown', (e) => this.onMouseDown(e));
	this.canvasElement.addEventListener('mousemove', (e) => this.onMouseMove(e));
	this.canvasElement.addEventListener('mouseup', (e) => this.onMouseUp(e));
  }

  onMouseDown(e) {
  }

  onMouseMove(e) {
	
  }

  onMouseUp(e) {
  }
}
