'use strict';

self.importScripts('matter.js');

self.onmessage = (msg) => {
  let canvas = msg.data.canvas;
  let context = canvas.getContext('2d');

  let engine = Matter.Engine.create({
	gravity: {
	  x: 0.0,
	  y: 0.0,
	},
  });

  let renderer = new Renderer(canvas, context);

  let sim = new AntSimulation(canvas, context, engine, renderer);
  sim.run();
};

// For now, the scaling factors are:
//
// * time in seconds
// * length in millimeters
// * mass in milligrams

let vec2 = (x, y) => ({x: x, y: y});

let degreesToRadians = (deg) => deg / 360.0 * 2.0 * Math.PI;

// angleClamp clamps `a` into the range `[0, 2ᴨ)`.
let angleClamp = (a) => {
  let reduced = a % (2.0 * Math.PI);
  if (reduced < 0.0) {
	return 2.0 * Math.PI + reduced;
  } else {
	return reduced;
  }
};

// shorterTurn returns an angle `Θ` such that `b + Θ = a` and `Θ ∈ [-ᴨ, ᴨ)`.
//
// Starting from angle b, you can turn either clockwise or counterclockwise to
// reach angle a.  shorterTurn gives you the shorter turn (positive angles
// clockwise, negative angles counterclockwise).
let shorterTurn = (a, b) => {
  let aClamp = angleClamp(a);
  let bClamp = angleClamp(b);

  // The primary turn is always in the range [-2ᴨ, 2ᴨ).  Decide whether or not
  // it is the smaller possible turn, and if not, return the complementary turn.
  let diff = aClamp - bClamp;
  let isSmallerTurn = Math.abs(diff) < Math.PI;

  if (isSmallerTurn) {
	return diff;
  } else {
	if (diff < 0.0) {
	  return 2.0 * Math.PI - diff;
	} else {
	  return -(2.0 * Math.PI - diff);
	}
  }
};

class AntSimulation {
  constructor(canvas, context, engine, renderer) {
	this.canvas = canvas;
	this.context = context;
	this.engine = engine;
	this.renderer = renderer;

	this.realLastTime = 0.0;
	this.simLastTime = 0.0;
	this.physicsLastTime = 0.0;

	this.anthill = Matter.Bodies.circle(0, 0, 50, {
	  isStatic: true,
	  isSensor: true,
	});
	Matter.Composite.add(this.engine.world, [this.anthill]);

	this.foodParticle = Matter.Bodies.circle(400, 400, 2, {
	  frictionAir: 0.5,
	  density: 1,
	});
	Matter.Composite.add(this.engine.world, [this.foodParticle]);

	this.ant = new Ant(this, 10, 10);
  }

  run() {
	requestAnimationFrame((newTime) => this.step(newTime));
  }

  step(realNewTime) {
	realNewTime /= 1000;
	let realDt = realNewTime - this.realLastTime;

	// Simulation update.
	let simDt = realNewTime - this.realLastTime;
	if (simDt > 1 / 60) {
	  simDt = 1 / 60;
	}
	let simNewTime = this.simLastTime + simDt;

	this.ant.simStep(simNewTime);
	if (this.foodParticle === null) {
	  let foodParticleX = Math.random() * (1000.0 - -1000.0) + -1000.0;
	  let foodParticleY = Math.random() * (1000.0 - -1000.0) + -1000.0;
	  this.foodParticle = Matter.Bodies.circle(400, 400, 2, {
		frictionAir: 0.5,
		density: 1,
	  });
	  Matter.Composite.add(this.engine.world, [this.foodParticle]);
	}

	// Physics update.
	let physDt = simDt;
	let physNewTime = this.physLastTime + physDt;
	Matter.Engine.update(this.engine, physDt * 1000);

	// Render frame.
	this.renderer.clear();
	this.renderer.drawRectCenteredAt(this.anthill.position, 100.0, 100.0);
	this.renderer.drawFoodParticleAt(this.foodParticle.position, this.foodParticle.angle);
	this.renderer.drawAntAt(this.ant.body.position, this.ant.body.angle, '#ff0000');
	if (this.ant.foodConstraint != null) {
	  this.renderer.drawLineAt(Matter.Constraint.pointAWorld(this.ant.foodConstraint), Matter.Constraint.pointBWorld(this.ant.foodConstraint), '#4a87d5', 2)
	}

	this.realLastTime = realNewTime;
	this.simLastTime = simNewTime;
	this.physLastTime = physNewTime;
	requestAnimationFrame((newTime) => this.step(newTime));
  }
}

let antStateStart = 'start';
let antStateUndefined = 'undefined';
let antStateFindFood = 'find-food';
let antStateAttachFood = 'attach-food';
let antStateRetrieveFood = 'retrieve-food';
let antStateDepositFood = 'deposit-food';

class Ant {
  constructor(sim, x, y) {
	this.sim = sim;
	this.simNewTime = 0.0;
	this.simLastTime = 0.0;
	this.simDT = 0.0;

	this.body = Matter.Bodies.circle(x, y, 1, {
	  frictionAir: 0.5,
	  density: 1,
	});
	Matter.Composite.add(this.sim.engine.world, [this.body]);

	this.topSpeed = 20;

	// The constraint we use for attaching to a food particle.
	this.foodConstraint = null;

	this.prevState = antStateStart;
	this.state = antStateFindFood;

	this.attachFoodTimerSet = false;
	this.attachFoodTimer = 0.0;

	this.depositFoodTimerSet = false;
	this.depositFoodTimer = 0.0;
  }

  // moveTowards is a low-level control primitive.  It moves the ant towards the
  // world position `target`, without pathing.  It returns true if the ant is
  // within `radius` of target.
  moveTowards(target, radius) {
	let headingX = Math.cos(this.body.angle);
	let headingY = Math.sin(this.body.angle);

	let toTargetX = target.x - this.body.position.x;
	let toTargetY = target.y - this.body.position.y;
	let toTargetLength = Math.sqrt(toTargetX * toTargetX + toTargetY * toTargetY);

	if (toTargetLength < radius) {
	  return true;
	}

	let toTargetAngle = Math.atan2(toTargetY, toTargetX);

	// Turn towards the target, with proportional control.
	let neededTurn = shorterTurn(toTargetAngle, this.body.angle);
	let desiredTurnControl = neededTurn * this.simDT;

	// We can turn at a max of 90 degrees per second.
	let maxTurnRate = degreesToRadians(90.0);
	let maxTurnControl = maxTurnRate * this.simDT;
	let turnControl = desiredTurnControl;
	if (desiredTurnControl >= maxTurnControl) {
	  turnControl = maxTurnControl;
	}
	if (desiredTurnControl < -maxTurnControl) {
	  turnControl = -maxTurnControl;
	}

	let positionControlX = 0.0;
	let positionControlY = 0.0;

	// If we are reasonably on-target, start accelerating on-axis.
	if (degreesToRadians(-20) <= neededTurn && neededTurn <= degreesToRadians(20)) {
	  positionControlX = headingX * this.topSpeed * this.simDT;
	  positionControlY = headingY * this.topSpeed * this.simDT;
	}

	// Update physics state.
	this.body.angle += turnControl;
	this.body.position.x += positionControlX;
	this.body.position.y += positionControlY;

	// We aren't within range.
	return false;
  }

  simStep(simNewTime) {
	this.simNewTime = simNewTime;
	this.simDT = this.simNewTime - this.simLastTime;
	if (this.simDT >= 1.0 / 60.0) {
	  this.simDT = 1.0 / 60.0;
	}

	let newState = antStateUndefined;

	if (this.state === antStateFindFood) {
	  if (this.moveTowards(this.sim.foodParticle.position, 10.0)) {
		newState = antStateAttachFood;
	  } else {
		newState = antStateFindFood;
	  }
	} else if (this.state === antStateAttachFood) {
	  if (this.prevState != this.state) {
		// State entry block
		this.foodConstraint = Matter.Constraint.create({
		  bodyA: this.body,
		  bodyB: this.sim.foodParticle,
		  stiffness: 0.7,
		});
		Matter.Composite.add(this.sim.engine.world, this.foodConstraint);

		this.attachFoodTimer = simNewTime + 1.0;
	  }

	  if (simNewTime >= this.attachFoodTimer) {
		this.attachFoodTimer = 0.0;
		newState = antStateRetrieveFood;
	  } else {
		newState = antStateAttachFood;
	  }
	} else if (this.state === antStateRetrieveFood) {
	  if (this.moveTowards(this.sim.anthill.position, 10.0)) {
		newState = antStateDepositFood;
	  } else {
		newState = antStateRetrieveFood;
	  }
	} else if (this.state === antStateDepositFood) {
	  if (this.prevState != this.state) {
		// State entry block.
		Matter.Composite.remove(this.sim.engine.world, this.foodConstraint);
		this.foodConstraint = null;

		Matter.Composite.remove(this.sim.engine.world, this.sim.foodParticle);
		this.sim.foodParticle = null;

		this.depositFoodTimer = simNewTime + 1.0;
	  }

	  if (simNewTime >= this.depositFoodTimer) {
		this.depositFoodTimer = 0.0;
		newState = antStateFindFood;
	  } else {
		newState = antStateDepositFood;
	  }
	} else {
	  throw new Error('Bad ant state');
	}

	if (newState === antStateUndefined) {
	  throw new Error('Bad state update');
	}

	this.prevState = this.state;
	this.state = newState;

	this.simLastTime = simNewTime;
  }
}

// meshCopy copies `mesh`.
//
// `mesh` is a list of vertices.
let meshCopy = (mesh) => {
  let copy = [];
  for (const v of mesh) {
	copy.push({x: v.x, y: v.y});
  }
  return v;
};

// meshRotate rotates all vertices of `mesh` by `angle`.
//
// `mesh` is a list of vertices.  `angle` is the rotation angle in radians.
// `offset` is a vector translation to apply after rotation.
//
// `outMesh` is the mesh to write the result into.  It must be the same length
// as `mesh`.  It may be the same object as `mesh`, if you do not need to
// preserve the original contents.
let meshAffineTransformInto = (mesh, angle, offset, outMesh) => {
  if (mesh.length != outMesh.length) {
	throw new Error("output mesh has wrong length");
  }

  let cos = Math.cos(angle);
  let sin = Math.sin(angle);
  for (let i = 0; i < mesh.length; i++) {
	let ix = mesh[i].x;
	let iy = mesh[i].y;
	outMesh[i].x = cos * ix - sin * iy + offset.x;
	outMesh[i].y = sin * ix + cos * iy + offset.y;
  }
};

class Renderer {
  constructor(canvas, context) {
	this.canvas = canvas;
	this.context = context;

	// Camera parameters (in world coordinates) that define the transform
	// between world and camera coordinates.  Height is always locked based on
    // width and the canvas aspect ratio.
	this.worldCenterX = 0.0;
	this.worldCenterY = 0.0;
	this.worldWidth = 1000.0;
  }

  worldHeight() {
	let aspectRatio = this.canvas.height / this.canvas.width;
	return this.worldWidth * aspectRatio;
  }

  cameraWidth() {
	return this.canvas.width;
  }

  cameraHeight() {
	return this.canvas.height;
  }

  // worldToCameraPoint transforms a point in world coordinates to a point in camera
  // coordinates.
  worldToCameraPoint(worldPoint) {
	let worldXMin = this.worldWidth / 2.0;
	let worldYMin = this.worldHeight() / 2.0;
	let x = (worldPoint.x - (this.worldCenterX - worldXMin)) / this.worldWidth * this.cameraWidth();
	let y = (worldPoint.y - (this.worldCenterY - worldYMin)) / this.worldHeight() * -1.0 * this.cameraHeight() + this.cameraHeight();
	return {x: x, y: y}
  }

  worldToCameraMesh(mesh, outMesh) {
	if (mesh.length != outMesh.length) {
	  throw new Error("output mesh has wrong length");
	}

	let worldXMin = this.worldWidth / 2.0;
	let worldYMin = this.worldHeight() / 2.0;
	for (let i = 0; i < mesh.length; i++) {
	  let ix = mesh[i].x;
	  let iy = mesh[i].y;
	  outMesh[i].x = (ix - (this.worldCenterX - worldXMin)) / this.worldWidth * this.cameraWidth();
	  outMesh[i].y = (iy - (this.worldCenterY - worldYMin)) / this.worldHeight() * -1.0 * this.cameraHeight() + this.cameraHeight();
	}
  }

  // Look at spot, in world coordinates.
  lookAt(x, y, width) {
	this.worldCenterX = x;
	this.worldCenterY = y;
	this.worldWidth = width;
  }

  clear() {
	this.context.globalCompositeOperation = 'source-over';
	this.context.clearRect(0, 0, this.canvas.width, this.canvas.height);
  }

  // drawAntAt draws an ant in world coordinates.
  drawAntAt(worldCenter, worldAngle, fillStyle) {
	let mesh = [
	  vec2(10.0, 0.0),
	  vec2(0.0, 3.0),
	  vec2(0.0, -3.0),
	];
	this.drawMeshAt(mesh, mesh, worldCenter, worldAngle, fillStyle, '#000000', 1);
  }

  // drawFoodParticle draws a food particle in world coordinates.
  drawFoodParticleAt(worldCenter, worldAngle) {
	let mesh = [
	  vec2(5.0, 0),
	  vec2(4.0, 4.0),
	  vec2(0.0, 5.0),
	  vec2(-4.0, 4.0),
	  vec2(-5.0, 0.0),
	  vec2(-4.0, -4.0),
	  vec2(0.0, -5.0),
	  vec2(4.0, -4.0),
	];
	this.drawMeshAt(mesh, mesh, worldCenter, worldAngle, '#72573a', '#000000', 1);
  }

  drawLineAt(worldA, worldB, strokeStyle, cameraStrokeWidth) {
	let cameraA = this.worldToCameraPoint(worldA);
	let cameraB = this.worldToCameraPoint(worldB);

	this.context.beginPath();
	this.context.moveTo(cameraA.x, cameraA.y);
	this.context.lineTo(cameraB.x, cameraB.y);

	this.context.lineWidth = cameraStrokeWidth;
	this.context.strokeStyle = strokeStyle;
	this.context.stroke();
  }

  // drawMeshAt draws the given mesh at `worldCenter` and `worldAngle`.
  //
  // `meshCopy` needs to be a mesh of the same size as `mesh`; it's scratch
  // space for transforming the mesh into camera coordinates.
  drawMeshAt(mesh, meshCopy, worldCenter, worldAngle, fillStyle, strokeStyle, cameraStrokeWidth) {
	if (mesh.length < 3) {
	  throw new Error('mesh is too short');
	}

	// Transform mesh to world coordinates, then camera coordinates.
	meshAffineTransformInto(mesh, worldAngle, worldCenter, meshCopy);
	this.worldToCameraMesh(meshCopy, meshCopy);

	this.context.beginPath();
	this.context.moveTo(meshCopy[0].x, meshCopy[0].y);
	for(let i = 1; i < meshCopy.length; i++) {
	  this.context.lineTo(meshCopy[i].x, meshCopy[i].y);
	}
	this.context.closePath();

	this.context.fillStyle = fillStyle;
	this.context.fill();

	this.context.lineWidth = cameraStrokeWidth;
	this.context.strokeStyle = strokeStyle;
	this.context.stroke();
  }

  // Draw a rectangle, in world coordinates.
  drawRectCenteredAt(worldCenter, width, height) {
	let cameraTR = this.worldToCameraPoint({x: worldCenter.x + width / 2.0, y: worldCenter.y + height / 2.0});
	let cameraTL = this.worldToCameraPoint({x: worldCenter.x - width / 2.0, y: worldCenter.y + height / 2.0});
	let cameraBL = this.worldToCameraPoint({x: worldCenter.x - width / 2.0, y: worldCenter.y - height / 2.0});
	let cameraBR = this.worldToCameraPoint({x: worldCenter.x + width / 2.0, y: worldCenter.y - height / 2.0});

	this.context.beginPath();
	this.context.moveTo(cameraTR.x, cameraTR.y);
	this.context.lineTo(cameraTL.x, cameraTL.y);
	this.context.lineTo(cameraBL.x, cameraBL.y);
	this.context.lineTo(cameraBR.x, cameraBR.y);
	this.context.closePath();

	this.context.fillStyle = '#ff0000';
	this.context.fill();

	this.context.lineWidth = 2;
	this.context.strokeStyle = '#000000';
	this.context.stroke();
  }
}
