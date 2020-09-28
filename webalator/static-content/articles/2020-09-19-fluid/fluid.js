'use strict';

class Grid {
  constructor(gridScale, rows, cols, canvas) {
	// What size is each grid cell?
	this.gridScale = gridScale;

	this.rows = rows;
	this.cols = cols;

	this.oldVelXBuf = new Float32Array(rows * cols);
	this.oldVelYBuf = new Float32Array(rows * cols);
	this.newVelXBuf = new Float32Array(rows * cols);
	this.newVelYBuf = new Float32Array(rows * cols);

	this.divVelBuf = new Float32Array(rows * cols);

	this.stirrerX = 0.0;
	this.stirrerY = 0.0;
	this.stirrerRadius = 10.0;

	this.numParticles = cols-2;
	this.particlePosX = new Float32Array(this.numParticles);
	this.particlePosY = new Float32Array(this.numParticles);
	for(let i = 1; i < cols-1; i++) {
	  this.particlePosX[i] = i+0.5;
	  this.particlePosY[i] = 50;
	}

	this.oldLiquidAmountBuf = new Float32Array(rows * cols);
	this.newLiquidAmountBuf = new Float32Array(rows * cols);
	for(let cx = 1; cx < 30; cx++) {
	  for(let cy = 30; cy < 50; cy++) {
		this.oldLiquidAmountBuf[cy * this.cols + cx] = 1.0;
	  }
	}

	this.canvas = canvas;
	this.context = canvas.getContext('2d');
	this.gl = canvas.getContext('webgl2');
  }

  velX(x, y) {
	return this.oldVelXBuf[y * this.cols + x];
  }

  velY(x, y) {
	return this.oldVelYBuf[y * this.cols + x];
  }

  setOldVelX(x, y, val) {
	this.oldVelXBuf[y * this.cols + x] = val;
  }

  setOldVelY(x, y, val) {
	this.oldVelYBuf[y * this.cols + x] = val;
  }

  setVelX(x, y, val) {
	this.newVelXBuf[y * this.cols + x] = val;
  }

  setVelY(x, y, val) {
	this.newVelYBuf[y * this.cols + x] = val;
  }

  divVel(x, y) {
	return this.divVelBuf[y * this.cols + x];
  }

  setDivVel(x, y, val) {
	this.divVelBuf[y * this.cols + x] = val;
  }

  liquidAmount(x, y) {
	return this.oldLiquidAmountBuf[y * this.cols + x];
  }

  setLiquidAmount(x, y, val) {
	this.newLiquidAmountBuf[y * this.cols + x] = val;
  }

  flipBuffers() {
	let tmpX = this.oldVelXBuf;
	this.oldVelXBuf = this.newVelXBuf;
	this.newVelXBuf = tmpX;

	let tmpY = this.oldVelYBuf;
	this.oldVelYBuf = this.newVelYBuf;
	this.newVelYBuf = tmpY;
  }

  flipLiquidBuf() {
	let tmpL = this.oldLiquidAmountBuf;
	this.oldLiquidAmountBuf = this.newLiquidAmountBuf;
	this.newLiquidAmountBuf = tmpL;
  }

  setBoundary() {
	this.setOldVelX(0, 0, 0.0);
	this.setOldVelY(0, 0, 0.0);

	this.setOldVelX(0, this.rows-1, 0.0);
	this.setOldVelY(0, this.rows-1, 0.0);

	this.setOldVelX(this.cols-1, 0, 0.0);
	this.setOldVelY(this.cols-1, 0, 0.0);

	this.setOldVelX(this.cols-1, this.rows-1, 0.0);
	this.setOldVelY(this.cols-1, this.rows-1, 0.0);

	// Bottom edge, top edge.  Replicate x, zero y.
	for(let cx = 1; cx < this.cols-1; cx++) {
	  let cy = 0;
	  this.setOldVelX(cx, cy, this.velX(cx, cy+1));
	  this.setOldVelY(cx, cy, 0.0);
	}
	for(let cx = 1; cx < this.cols-1; cx++) {
	  let cy = this.rows-1;
	  this.setOldVelX(cx, cy, this.velX(cx, cy-1));
	  this.setOldVelY(cx, cy, 0.0);
	}

	// Left edge, right edge.  Zero x, replicate y.
	for(let cy = 1; cy < this.rows-1; cy++) {
	  let cx = 0;
	  this.setOldVelX(cx, cy, 0.0);
	  this.setOldVelY(cx, cy, this.velY(cx+1, cy));
	}
	for(let cy = 1; cy < this.rows-1; cy++) {
	  let cx = this.cols-1;
	  this.setOldVelX(cx, cy, 0.0);
	  this.setOldVelY(cx, cy, this.velY(cx-1, cy));
	}
  }

  forceStirrerVelocity() {
	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.rows-1; cy++) {

		let x = cx + 0.5 - this.stirrerX;
		let y = cy + 0.5 - this.stirrerY;
		let radius = x*x + y*y;
		if(radius < this.stirrerRadius*this.stirrerRadius) {
		  this.setOldVelX(cx, cy, 0.0);
		  this.setOldVelY(cx, cy, 5.0);
		}
	  }
	}
  }

  advect(dt) {
	// Semi-Lagrangian advection: Figure out where the point at the center of
	// the cell came from, and then interpolate between the four cells
	// surrounding that point.

	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.rows-1; cy++) {
		// Compute origin point for this cell.
		let ox = (cx+0.5) - this.velX(cx, cy) * dt / this.gridScale;
		let oy = (cy+0.5) - this.velY(cx, cy) * dt / this.gridScale;

		// Clamp the origin point into the non-boundary cells.  Since we assume
		// a closed boundary, fluid can't be coming from outside.
		if(ox < 1.0) {
		  ox = 1.0;
		}
		if(ox >= this.cols-1) {
		  ox = this.cols - 1 - 0.00001;
		}
		if(oy < 1.0) {
		  oy = 1.0;
		}
		if(oy >= this.rows-1) {
		  oy = this.rows - 1 - 0.00001;
		}

		let ocx = Math.floor(ox);
		let ocy = Math.floor(oy);

		// Compute the velocities at the four corners of the origin cell.

		let tlvx = (this.velX(ocx, ocy) + this.velX(ocx-1, ocy) + this.velX(ocx, ocy+1) + this.velX(ocx-1, ocy+1)) / 4.0;
		let tlvy = (this.velY(ocx, ocy) + this.velY(ocx-1, ocy) + this.velY(ocx, ocy+1) + this.velY(ocx-1, ocy+1)) / 4.0;

		let trvx = (this.velX(ocx, ocy) + this.velX(ocx+1, ocy) + this.velX(ocx, ocy+1) + this.velX(ocx+1, ocy+1)) / 4.0;
		let trvy = (this.velY(ocx, ocy) + this.velY(ocx+1, ocy) + this.velY(ocx, ocy+1) + this.velY(ocx+1, ocy+1)) / 4.0;

		let blvx = (this.velX(ocx, ocy) + this.velX(ocx-1, ocy) + this.velX(ocx, ocy-1) + this.velX(ocx-1, ocy-1)) / 4.0;
		let blvy = (this.velY(ocx, ocy) + this.velY(ocx-1, ocy) + this.velY(ocx, ocy-1) + this.velY(ocx-1, ocy-1)) / 4.0;

		let brvx = (this.velX(ocx, ocy) + this.velX(ocx+1, ocy) + this.velX(ocx, ocy-1) + this.velX(ocx+1, ocy-1)) / 4.0;
		let brvy = (this.velY(ocx, ocy) + this.velY(ocx+1, ocy) + this.velY(ocx, ocy-1) + this.velY(ocx+1, ocy-1)) / 4.0;

		// Interpolate between the cell corners to compute the new origin
		// velocity.
		let sX = ox - ocx;
		let sY = oy - ocy;
		let newVX = (1.0 - sX) * (1.0 - sY) * blvx + sX * (1.0 - sY) * brvx + (1.0 - sX) * sY * tlvx + sX * sY * trvx;
		let newVY = (1.0 - sX) * (1.0 - sY) * blvy + sX * (1.0 - sY) * brvy + (1.0 - sX) * sY * tlvy + sX * sY * trvy;
		this.setVelX(cx, cy, newVX);
		this.setVelY(cx, cy, newVY);
	  }
	}
  }

  killDivergence() {
	// Compute divergence
	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.rows-1; cy++) {
		let div = (this.velX(cx+1,cy) - this.velX(cx-1, cy)) / 2 +
			(this.velY(cx, cy+1) - this.velY(cx, cy-1)) / 2;
		this.setDivVel(cx, cy, div);
	  }
	}

	// Compute gradient of divergence.
	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.rows-1; cy++) {
		let gradDivVelX = (this.divVel(cx+1, cy) - this.divVel(cx-1, cy)) / 2;
		let gradDivVelY = (this.divVel(cx, cy+1) - this.divVel(cx, cy-1)) / 2;
		this.setVelX(cx, cy, this.velX(cx, cy) + 0.9 * gradDivVelX);
		this.setVelY(cx, cy, this.velY(cx, cy) + 0.9 * gradDivVelY);
	  }
	}
  }

  moveLiquid(dt) {
	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.cols-1; cy++) {
		this.setLiquidAmount(cx, cy, this.liquidAmount(cx, cy));
	  }
	}
  }

  moveParticles(dt) {
	for(let i = 0; i < this.numParticles; ++i) {
	  let x = this.particlePosX[i];
	  let y = this.particlePosY[i];

	  if(x < 1.0) {
		x = 1.0;
	  }
	  if(x >= this.cols - 1) {
		x = this.cols - 1 - 0.00001;
	  }
	  if(y < 1.0) {
		y = 1.0;
	  }
	  if(y >= this.rows - 1) {
		y = this.rows - 1 - 0.00001;
	  }

	  let cx = Math.floor(x);
	  let cy = Math.floor(y);

	  let vx = this.velX(cx, cy);
	  let vy = this.velY(cx, cy) - 1.0;

	  x += dt * vx;
	  y += dt * vy;

	  x += (2*Math.random()-1.0) * 0.025;
	  y += (2*Math.random()-1.0) * 0.025;

	  if(x < 1.0) {
		x = 1.0;
	  }
	  if(x >= this.cols - 1) {
		x = this.cols - 1 - 0.00001;
	  }
	  if(y < 1.0) {
		y = 1.0;
	  }
	  if(y >= this.rows - 1) {
		y = this.rows - 1 - 0.00001;
	  }

	  this.particlePosX[i] = x;
	  this.particlePosY[i] = y;
	}
  }

  draw() {
	let gridCellSizePx = this.canvas.width / this.cols;

	this.context.clearRect(0, 0, this.canvas.width, this.canvas.height);

	// Draw liquid
	let liquidSum = 0.0;
	for(let cx = 1; cx < this.cols-1; cx++) {
	  for(let cy = 1; cy < this.rows-1; cy++) {
		liquidSum += this.liquidAmount(cx, cy);
		if(this.liquidAmount(cx, cy) > 0.0) {
		  this.context.fillStyle = 'rgba(0, 0, 255, 1)';
		  this.context.fillRect(cx * gridCellSizePx, this.canvas.height - (cy * gridCellSizePx), gridCellSizePx, gridCellSizePx);
		}
	  }
	}
	this.context.font = '50px serif';
	this.context.fillText(''+liquidSum, 10, 50);

	// Draw vector field.
	for(let cx = 0; cx < this.cols; cx+=5) {
	  for(let cy = 0; cy < this.rows; cy+=5) {
		let vecBaseX = cx * gridCellSizePx + 0.5 * gridCellSizePx;
		let vecBaseY = this.canvas.height - (cy * gridCellSizePx + 0.5 * gridCellSizePx);

		let vecX = this.velX(cx, cy) * gridCellSizePx;
		let vecY = -this.velY(cx, cy) * gridCellSizePx;

		this.context.strokeStyle = 'rgba(0, 0, 0, 0.5)';
		this.context.beginPath();
		this.context.moveTo(vecBaseX, vecBaseY);
		this.context.lineTo(vecBaseX+vecX, vecBaseY+vecY);
		this.context.stroke();
	  }
	}

	// Draw stirrer
	this.context.strokeStyle = 'rgba(255, 0, 0, 0.5)';
	this.context.beginPath();
	this.context.arc(this.stirrerX * gridCellSizePx,
					 this.canvas.height - this.stirrerY * gridCellSizePx,
					 this.stirrerRadius * gridCellSizePx,
					 0.0,
					 2 * Math.PI);
	this.context.stroke();

	// Draw particles
	this.context.strokeStyle = 'rgba(0, 0, 0, 1)';
	this.context.fillStyle = 'rgba(0, 255, 0, 0.5)';
	for(let i = 0; i < 100; i++) {
	  this.context.beginPath();
	  this.context.arc(this.particlePosX[i] * gridCellSizePx,
					   this.canvas.height - this.particlePosY[i] * gridCellSizePx,
					   0.5 * gridCellSizePx,
					   0.0,
					   2 * Math.PI);
	  this.context.fill();
	  this.context.stroke();
	}

  }

  run() {
	// The system dynamics are always simulated with a fixed timestep, iterated
	// enough times to bring us up to the current simulation time.
	let physicsDT = 0.05;

	let curTime = performance.now() / 1000.0;
	let physicsCurTime = 0.0;

	let step = (newTime) => {
	  newTime /= 1000.0;

	  let dt = newTime - curTime;

	  // Clamp our elapsed real time, to prevent a long first frame after pausing
	  // in the debugger.
	  if(dt > 0.1) {
		dt = 0.1;
	  }

	  let physicsTargetTime = physicsCurTime + dt;
	  while(physicsCurTime < physicsTargetTime) {

		this.stirrerX = this.cols / 2.0 + this.cols / 2.0 * Math.cos(physicsCurTime / 10.0);
		this.stirrerY = 5;

		this.forceStirrerVelocity();

		this.setBoundary();
		this.advect(physicsDT);
		this.flipBuffers();

		// this.liquidGravity(physicsDT);
		// this.flipBuffers();

		for(let i = 0; i < 60; i++) {
		  this.forceStirrerVelocity();
		  this.setBoundary();
		  this.killDivergence();
		  this.flipBuffers();
		}

		this.moveLiquid(physicsDT);
		this.flipLiquidBuf();

		this.moveParticles(physicsDT);

		physicsCurTime += physicsDT;
	  }

	  this.draw();

	  curTime = newTime;
	  requestAnimationFrame(step);
	};

	requestAnimationFrame(step);
  }
}

self.onmessage = (msg) => {
  let canvas = msg.data.canvas;
  let grid = new Grid(0.1, 60, 60, canvas);
  grid.run();
}
