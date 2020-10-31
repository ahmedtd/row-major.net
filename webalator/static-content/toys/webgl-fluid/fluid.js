'use strict';

const doNothingVertexShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

in vec2 vertexPos;
void main() {
    gl_Position = vec4(vertexPos, 0.0, 1.0);
}
`;

const boundaryConditionFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform vec2 stirrerPos;
uniform float stirrerRadius;
uniform float metersPerCell;

uniform sampler2D oldVelocity;

out vec2 newVelocity;

void main() {
  // In this shader, our fragment coordinates are cell centers.
  if(length(gl_FragCoord.xy - stirrerPos) < stirrerRadius) {
    newVelocity.x = 0.0;
    newVelocity.y = 1.0 / metersPerCell;
    return;
  }

  newVelocity = texelFetch(oldVelocity, ivec2(gl_FragCoord.xy), 0).xy;
}

`

const advectFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform float dt;
uniform sampler2D oldVelocity;

out vec2 newVelocity;

void main() {
  vec2 velocitySize = vec2(textureSize(oldVelocity, 0));

  // In this shader, our fragment coordinates are cell coordinates.
  vec2 coord = gl_FragCoord.xy;
  vec2 cell = floor(coord);
  vec2 cellCenter = floor(cell) + vec2(0.5, 0.5);

  vec2 velocity = texelFetch(oldVelocity, ivec2(cell), 0).xy;

  // Compute origin point for the packet currently at the center of this cell.
  vec2 origin = cellCenter - velocity * dt;

  // Clamp to non-boundary cells; fluid can't cross the grid boundary.
  origin = clamp(origin, vec2(1.0,1.0), velocitySize - vec2(1.0,1.0));

  ivec2 originCell = ivec2(floor(origin));

  // Compute velocities at the four corners of the origin cell.
  vec2 velTL = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,1), 0).xy
  );
  vec2 velTR = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,1), 0).xy
  );
  vec2 velBL = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,-1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,-1), 0).xy
  );
  vec2 velBR = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,-1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,-1), 0).xy
  );

  // Interpolate between the cell corners to compute the new velocity.
  vec2 s = origin - vec2(originCell);

  newVelocity =
    (1.0-s.x) * (1.0-s.y) * velBL +
    s.x * (1.0-s.y) * velBR +
    (1.0-s.x) * s.y * velTL +
    s.x * s.y * velTR;
}
`;

const removeDivergenceFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform sampler2D oldVelocity;

out vec2 newVelocity;

vec2 gradientOfDivergenceAt(sampler2D field, ivec2 cellCC) {
  ivec2 size = textureSize(field, 0);

  ivec2 cellSW = cellCC + ivec2(-1,-1);
  ivec2 cellWW = cellCC + ivec2(-1,+0);
  ivec2 cellNW = cellCC + ivec2(-1,+1);
  ivec2 cellSS = cellCC + ivec2(+0,-1);
  ivec2 cellNN = cellCC + ivec2(+0,+1);
  ivec2 cellSE = cellCC + ivec2(+1,-1);
  ivec2 cellEE = cellCC + ivec2(+1,+0);
  ivec2 cellNE = cellCC + ivec2(+1,+1);

  vec2 fieldSW;
  vec2 fieldWW;
  vec2 fieldNW;
  vec2 fieldSS;
  vec2 fieldCC;
  vec2 fieldNN;
  vec2 fieldSE;
  vec2 fieldEE;
  vec2 fieldNE;

  if(cellCC.x == 0 && cellCC.y == 0) {
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldNE = texelFetch(field, cellNE, 0).xy;
    fieldSW = vec2(0.0, 0.0);
    fieldWW = vec2(0.0, fieldCC.y);
    fieldNW = vec2(0.0, fieldNN.y);
    fieldSS = vec2(fieldCC.x, 0.0);
    fieldSE = vec2(fieldEE.x, 0.0);
  } else if(cellCC.x == 0 && cellCC.y == size.y-1) {
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldSE = texelFetch(field, cellSE, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldSW = vec2(0.0, fieldSS.y);
    fieldWW = vec2(0.0, fieldCC.y);
    fieldNW = vec2(0.0, 0.0);
    fieldNN = vec2(fieldCC.x, 0.0);
    fieldNE = vec2(fieldEE.x, 0.0);
  } else if(cellCC.x == size.x-1 && cellCC.y == 0) {
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldNW = texelFetch(field, cellNW, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldSW = vec2(fieldWW.x, 0.0);
    fieldSS = vec2(fieldCC.x, 0.0);
    fieldSE = vec2(0.0, 0.0);
    fieldEE = vec2(0.0, fieldCC.y);
    fieldNE = vec2(0.0, fieldNN.y);
  } else if(cellCC.x == size.x-1 && cellCC.y == size.y-1) {
    fieldSW = texelFetch(field, cellSW, 0).xy;
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNW = vec2(fieldWW.x, 0.0);
    fieldNN = vec2(fieldCC.x, 0.0);
    fieldSE = vec2(0.0, fieldSS.y);
    fieldEE = vec2(0.0, fieldEE.y);
    fieldNE = vec2(0.0, 0.0);
  } else if(cellCC.x == 0) {
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldSE = texelFetch(field, cellSE, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldNE = texelFetch(field, cellNE, 0).xy;
    fieldSW = vec2(0.0, fieldSS.y);
    fieldWW = vec2(0.0, fieldCC.y);
    fieldNW = vec2(0.0, fieldNN.y);
  } else if(cellCC.y == 0) {
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldNW = texelFetch(field, cellNW, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldNE = texelFetch(field, cellNE, 0).xy;
    fieldSW = vec2(fieldWW.x, 0.0);
    fieldSS = vec2(fieldSS.x, 0.0);
    fieldSE = vec2(fieldEE.x, 0.0);
  } else if(cellCC.x == size.x-1) {
    fieldSW = texelFetch(field, cellSW, 0).xy;
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldNW = texelFetch(field, cellNW, 0).xy;
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldSE = vec2(0.0, fieldSE.y);
    fieldEE = vec2(0.0, fieldCC.y);
    fieldNE = vec2(0.0, fieldNN.y);
  } else if(cellCC.y == size.y-1) {
    fieldSW = texelFetch(field, cellSW, 0).xy;
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldSE = texelFetch(field, cellSE, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldNW = vec2(fieldWW.x, 0.0);
    fieldNN = vec2(fieldCC.x, 0.0);
    fieldNE = vec2(fieldEE.x, 0.0);
  } else {
    fieldSW = texelFetch(field, cellSW, 0).xy;
    fieldWW = texelFetch(field, cellWW, 0).xy;
    fieldNW = texelFetch(field, cellNW, 0).xy;
    fieldSS = texelFetch(field, cellSS, 0).xy;
    fieldCC = texelFetch(field, cellCC, 0).xy;
    fieldNN = texelFetch(field, cellNN, 0).xy;
    fieldSE = texelFetch(field, cellSE, 0).xy;
    fieldEE = texelFetch(field, cellEE, 0).xy;
    fieldNE = texelFetch(field, cellNE, 0).xy;
  }

  float divergenceSW =
    (0.5*(fieldSS.x + fieldCC.x) - 0.5*(fieldSW.x + fieldWW.x)) +
    (0.5*(fieldWW.y + fieldCC.y) - 0.5*(fieldSW.y + fieldSS.y));
  float divergenceNW =
    (0.5*(fieldCC.x + fieldNN.x) - 0.5*(fieldWW.x + fieldNW.x)) +
    (0.5*(fieldNW.y + fieldNN.y) - 0.5*(fieldWW.y + fieldCC.y));
  float divergenceSE =
    (0.5*(fieldSE.x + fieldEE.x) - 0.5*(fieldSS.x + fieldCC.x)) +
    (0.5*(fieldCC.y + fieldEE.y) - 0.5*(fieldSS.y + fieldSE.y));
  float divergenceNE =
    (0.5*(fieldEE.x + fieldNE.x) - 0.5*(fieldCC.x + fieldNN.x)) +
    (0.5*(fieldNN.y + fieldNE.y) - 0.5*(fieldCC.y + fieldEE.y));

  float divergenceNN = 0.5*(divergenceNW+divergenceNE);
  float divergenceSS = 0.5*(divergenceSW+divergenceSE);
  float divergenceWW = 0.5*(divergenceSW+divergenceNW);
  float divergenceEE = 0.5*(divergenceSE+divergenceNE);

  vec2 gradient = vec2(
    divergenceEE - divergenceWW,
    divergenceNN - divergenceSS
  );

  return gradient;
}

void main() {
  // In this shader, our fragment coordinates are cell centers.
  vec2 gradient = gradientOfDivergenceAt(oldVelocity, ivec2(gl_FragCoord.xy));

  newVelocity = texelFetch(oldVelocity, ivec2(gl_FragCoord.xy), 0).xy + 0.9 * gradient;
}
`

const renderFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform ivec2 viewport;
uniform sampler2D oldVelocity;

out vec4 fragColor;

float onNeedle(vec2 point, vec2 start, vec2 end, float widthBase, float aaBorder) {
  if(length(end - start) < 0.001) {
    return 0.0;
  }

  float lengthParallel = dot(normalize(end - start), point - start);
  vec2 parallel = lengthParallel * normalize(end - start);
  vec2 perpendicular = (point - start) - parallel;

  float t = lengthParallel / length(end - start);

  if(t < 0.0 || t > 1.0) {
    return 0.0;
  }

  float width = t * 0.0 + (1.0-t) * widthBase;

  if(length(perpendicular) < width) {
    return 1.0;
  } else if(length(perpendicular) < width + aaBorder) {
    float s = (length(perpendicular) - width) / (aaBorder);
    return 1.0 - s;
  } else {
    return 0.0;
  }
}

void main() {
  ivec2 velocitySize = textureSize(oldVelocity, 0);
  vec2 pxPerGrid = vec2(viewport) / vec2(velocitySize);

  vec2 velocityGridFragCoord = gl_FragCoord.xy / pxPerGrid;

  float maxOnNeedle = 0.0;
  for(int dx = -3; dx <= 3; dx++) {
    for(int dy = -3; dy <= 3; dy++) {
      ivec2 cell;
      cell.x = int(velocityGridFragCoord.x) + dx;
      cell.y = int(velocityGridFragCoord.y) + dy;

      if(cell.x < 0 || cell.x >= velocitySize.x || cell.y < 0 || cell.y >= velocitySize.y) {
        continue;
      }

      vec2 velocity = texelFetch(oldVelocity, cell, 0).xy;
      vec2 cellCenter = vec2(cell) + vec2(0.5, 0.5);

      float curOnNeedle = onNeedle(
        gl_FragCoord.xy,
        cellCenter * pxPerGrid,
        (cellCenter + velocity) * pxPerGrid,
        2.0,
        0.5
      );

      if(curOnNeedle > maxOnNeedle) {
        maxOnNeedle = curOnNeedle;
      }
    }
  }

  fragColor = vec4(0.0, 0.0, 0.0, maxOnNeedle);
}
`;

function compileShaderObject(gl, type, text) {
  let shaderObject = gl.createShader(type);
  gl.shaderSource(shaderObject, text);
  gl.compileShader(shaderObject);

  if(!gl.getShaderParameter(shaderObject, gl.COMPILE_STATUS)) {
    throw new Error("Error compiling shader: " + gl.getShaderInfoLog(shaderObject));
  }

  return shaderObject;
}

function buildShaderProgram(gl, vertexShaders, fragmentShaders) {
  let vertexObjects = vertexShaders.map((x) => compileShaderObject(gl, gl.VERTEX_SHADER, x));
  let fragmentObjects = fragmentShaders.map((x) => compileShaderObject(gl, gl.FRAGMENT_SHADER, x));

  let program = gl.createProgram();
  vertexObjects.forEach((x) => gl.attachShader(program, x));
  fragmentObjects.forEach((x) => gl.attachShader(program, x));
  gl.linkProgram(program);

  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error("Error linking shader program: " + gl.getProgramInfoLog(program));
  }

  return program;
}

class Grid {
  constructor(gridScale, rows, cols, canvas) {
	// What size is each grid cell?
	this.gridScale = gridScale;

	this.rows = rows;
	this.cols = cols;

	this.oldVelBuf = new Float32Array(rows * cols * 2);
	this.newVelBuf = new Float32Array(rows * cols * 2);
	for(let cx = 0; cx < this.cols; cx++) {
	  for(let cy = 0; cy < this.rows; cy++) {
		this.oldVelBuf[cy * this.cols * 2 + cx * 2 + 0] = cx / this.cols;
		this.oldVelBuf[cy * this.cols * 2 + cx * 2 + 1] = cy / this.rows;
	  }
	}

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
	// this.context = canvas.getContext('2d');
	this.gl = canvas.getContext('webgl2');

	// Enable this extension so we can render to a floating-point texture.
	if(this.gl.getExtension('EXT_color_buffer_float') == null) {
	  throw new Error('Cannot load EXT_color_buffer_float extension');
	}

	// Initialize vertex data.
	this.vertexBuf = this.gl.createBuffer();
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.bufferData(this.gl.ARRAY_BUFFER, new Float32Array([
      1.0,  1.0,
      -1.0, 1.0,
      1.0,  -1.0,
      -1.0, -1.0,
    ]), this.gl.STATIC_DRAW);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	// Initialize the old velocity cells texture.
	//
	// In the velocity texture, all velocities are expressed as cells per second.
	this.oldVelocityTexture = this.gl.createTexture();
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);
	this.gl.texImage2D(this.gl.TEXTURE_2D, 0, this.gl.RG32F, this.cols, this.rows,
					   0, this.gl.RG, this.gl.FLOAT, this.oldVelBuf, 0);
	this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.gl.CLAMP_TO_EDGE);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.gl.CLAMP_TO_EDGE);
    this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	// Initialize the new velocity cells texture.
	this.newVelocityTexture = this.gl.createTexture();
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.newVelocityTexture);
	this.gl.texImage2D(this.gl.TEXTURE_2D, 0, this.gl.RG32F, this.cols, this.rows,
					   0, this.gl.RG, this.gl.FLOAT, this.newVelBuf, 0);
	this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.gl.CLAMP_TO_EDGE);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.gl.CLAMP_TO_EDGE);
    this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	// A framebuffer that sets our new velocity cells texture as the render target.
	this.velocityFB = this.gl.createFramebuffer();
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.framebufferTexture2D(this.gl.FRAMEBUFFER,
								 this.gl.COLOR_ATTACHMENT0,
								 this.gl.TEXTURE_2D,
								 this.newVelocityTexture,
								 0);
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);


	this.boundaryConditionProgram = buildShaderProgram(this.gl,
													   [doNothingVertexShader],
													   [boundaryConditionFragmentShader]);
	this.boundaryConditionVertexLoc = this.gl.getAttribLocation(this.boundaryConditionProgram, 'vertexPos');
	this.boundaryConditionStirrerPosLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'stirrerPos');
	this.boundaryConditionStirrerRadiusLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'stirrerRadius');
	this.boundaryConditionOldVelocityLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'oldVelocity');
	this.boundaryConditionMetersPerCellLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'metersPerCell');

	this.advectProgram = buildShaderProgram(this.gl,
											[doNothingVertexShader],
											[advectFragmentShader]);
	this.advectVertexLoc = this.gl.getAttribLocation(this.advectProgram, 'vertexPos');
	this.advectDTLoc = this.gl.getUniformLocation(this.advectProgram, 'dt');
	this.advectOldVelocityLoc = this.gl.getUniformLocation(this.advectProgram,
															 'oldVelocity');

	this.removeDivergenceProgram = buildShaderProgram(this.gl,
													  [doNothingVertexShader],
													  [removeDivergenceFragmentShader]);
	this.removeDivergenceVertexLoc = this.gl.getAttribLocation(this.removeDivergenceProgram, 'vertexPos');
	this.removeDivergenceOldVelocityLoc = this.gl.getUniformLocation(this.removeDivergenceProgram, 'oldVelocity');

	this.renderProgram = buildShaderProgram(this.gl,
											[doNothingVertexShader],
											[renderFragmentShader]);
	this.renderVertexLoc = this.gl.getAttribLocation(this.renderProgram, 'vertexPos');
	this.renderViewportLoc = this.gl.getUniformLocation(this.renderProgram, 'viewport');
	this.renderOldVelocityLoc = this.gl.getUniformLocation(this.renderProgram, 'oldVelocity');
  }

  swapVelocityTextures() {
	let tmp = this.oldVelocityTexture;
	this.oldVelocityTexture = this.newVelocityTexture;
	this.newVelocityTexture = tmp;

	// Update the framebuffer target as well.
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.framebufferTexture2D(this.gl.FRAMEBUFFER,
								 this.gl.COLOR_ATTACHMENT0,
								 this.gl.TEXTURE_2D,
								 this.newVelocityTexture,
								 0);
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);
  }

  runBoundaryConditionProgram() {
	this.gl.useProgram(this.boundaryConditionProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.boundaryConditionVertexLoc);
	this.gl.vertexAttribPointer(this.boundaryConditionVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.boundaryConditionOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform2f(this.boundaryConditionStirrerPosLoc, this.stirrerX, this.stirrerY);
	this.gl.uniform1f(this.boundaryConditionStirrerRadiusLoc, 10.0);

	this.gl.uniform1f(this.boundaryConditionMetersPerCellLoc, this.gridScale);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runAdvectProgram(dt) {
	this.gl.useProgram(this.advectProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.advectVertexLoc);
	this.gl.vertexAttribPointer(this.advectVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.advectOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform1f(this.advectDTLoc, dt);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runRemoveDivergenceProgram() {
	this.gl.useProgram(this.removeDivergenceProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.removeDivergenceVertexLoc);
	this.gl.vertexAttribPointer(this.removeDivergenceVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.removeDivergenceOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runRenderProgram() {
	this.gl.useProgram(this.renderProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.renderVertexLoc);
	this.gl.vertexAttribPointer(this.renderVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.renderOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform2i(this.renderViewportLoc, this.canvas.width, this.canvas.height);

	this.gl.viewport(0, 0, this.canvas.width, this.canvas.height);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
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
		this.runBoundaryConditionProgram();
		this.swapVelocityTextures();

		this.runAdvectProgram(physicsDT);
		this.swapVelocityTextures();

		for(let i = 0; i < this.cols; i++) {
		  this.runBoundaryConditionProgram();
		  this.swapVelocityTextures();

		  this.runRemoveDivergenceProgram();
		  this.swapVelocityTextures();
		}

		this.stirrerX = this.cols / 2.0 + this.cols / 2.0 * Math.cos(physicsCurTime / 10.0);
		this.stirrerY = 5;

		physicsCurTime += physicsDT;
	  }

	  this.runRenderProgram();

	  curTime = newTime;
	  requestAnimationFrame(step);
	};

	requestAnimationFrame(step);
  }
}

self.onmessage = (msg) => {
  let canvas = msg.data.canvas;
  let grid = new Grid(1.0, 20, 20, canvas);
  grid.run();
}
