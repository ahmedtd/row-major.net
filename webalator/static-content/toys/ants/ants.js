'use strict';

self.importScripts('matter.js');

self.onmessage = (msg) => {
  let canvas = msg.data.canvas;
  let context = canvas.getContext('2d');

  let engine = Matter.Engine.create();
  let render = Matter.Render.create({
	canvas: canvas,
	context: context,
	engine: engine,
  });

  console.log('step');

  let sim = new AntSimulation(canvas, context, engine, render);
  sim.run();
};

class AntSimulation {
  constructor(canvas, context, engine, render) {
	this.canvas = canvas;
	this.context = context;
	this.engine = engine;
	this.render = render;

	let rect = Matter.Bodies.rectangle(400, 200, 80, 80)
	Matter.Composite.add(this.engine.world, [rect]);
  }

  run() {
	requestAnimationFrame((newTime) => this.step(newTime));
  }

  step(newTime) {
	Matter.Engine.update(this.engine);
	Matter.Render.world(this.render, newTime);

	requestAnimationFrame((newTime) => this.step(newTime));
  }
}
