'use strict';

// Utilities for working with a rigid, scalable viewport.  Used for
// drawing the current-state visualization.  (Canvas transforms affect
// things like stroke width, but I'm trying to achieve a pen-like
// effect for a technical drawing.  No matter what scale I'm currently
// drawing at, my stroke width and fill characteristics should stay
// the same.

let viewport_make = (pixels_per_unit,
                     pixel_width
                     ) => ({pixels_per_unit: pixels_per_unit,
                                  center: center});

let viewport_fit = (pixel_width, pixel_height,
                    unit_width, unit_height
                    unit_center) => {
    let min = Math.min;
    let pixels_per_unit = min(pixel_width, pixel_height) / min(unit_width, unit_height);
    return {pixels_per_unit: pixels_per_unit,
            pixel_width: pixel_width,
            pixel_height: pixel_height,
            unit_width: unit_width,
            unit_height: unit_height,
            unit_center: unit_center};                                                     
};

let viewport_apply_point = (viewport, p) => {
    let screen_center = [viewport.pixel_width / 2,
                         viewport.pixel_height / 2];
    return [viewport.pixels_per_unit * p[0] - (viewport.unit_center[0],
            viewport.pixels_per_unit * p[1] - viewport.unit_center[1]];
};

// First, let's set up the physical system's dynamics.
//
//

let ball_make = (mass, pos, vel) => ({mass: mass,
                                      pos: pos,
                                      vel: vel});

let ball_make_random = (mass, limit_lo, limit_hi) => ({mass: mass,
                                                       pos: limit_lo + Math.random() * (limit_hi - limit_lo),
                                                       vel: 0.0});

let gravity_acc = -9.8;

// Symplectic Euler
let ball_integrate = (ball, force, dt) => {
    let acc = force / ball.mass + gravity_acc;
    // acc /= 10;
    // let new_vel = ball.vel + dt * acc;
    // let new_pos = ball.pos + dt * new_vel;

    // ball.pos = new_pos;
    // ball.vel = new_vel;

    let new_pos = ball.pos + dt * acc / 10;
    ball.pos = new_pos;
};


// Next, set up our representation for function approximations
//
// We will use a one-dimensional dense cosine basis.

let cos = Math.cos;
let pi = Math.PI;

let cosine_basis_make_zero = (min_x, max_x, basis_size) => {
    return {min_x: min_x,
            max_x: max_x,
            basis_size: basis_size,
            basis_weights: new Float32Array(basis_size)};
};

let cosine_basis_value = (basis, x) => {
    let x_norm = (x - basis.min_x) / (basis.max_x - basis.min_x);

    let value = 0;
    for(let i = 0; i < basis.basis_size; ++i) {
        value += basis.basis_weights[i] * cos(pi * i * x_norm);
    }

    return value;
};

let cosine_basis_refine = (basis, x, delta, base_step) => {
    let x_norm = (x - basis.min_x) / (basis.max_x - basis.min_x);

    for(let i = 0; i < basis.basis_size; ++i) {
        let step = base_step / (1 + i);
        // let step = base_step;
        basis.basis_weights[i] += step * delta * cos(pi * i * x_norm);
    }
};

// Now, define our control problem

let ball_pos_min = 0.0;
let ball_pos_max = 1.0;

let end_episode_p = (ball) =>
    (ball.pos < 0.0) || (ball.pos > 1.0);

let reward = (ball, dt) =>
    ((ball.pos >= 0.45) && (ball.pos <= 0.55))
    ? 100.0 * dt
    : -10.0 * dt;

let policy_make = (basis_size, average_reward, step_alpha, step_beta) =>
    ({reward_fn_force_on: cosine_basis_make_zero(ball_pos_min,
                                                 ball_pos_max,
                                                 basis_size),
      reward_fn_force_off: cosine_basis_make_zero(ball_pos_min,
                                                  ball_pos_max,
                                                  basis_size),
      average_reward: average_reward,
      step_alpha: step_alpha,
      step_beta: step_beta});

// Select an epsilon-greedy action according to the policy and the
// current state of the system.
let policy_apply = (policy, ball_pos) => {
    let est_value_on = cosine_basis_value(policy.reward_fn_force_on, ball_pos);
    let est_value_off = cosine_basis_value(policy.reward_fn_force_off, ball_pos);

    let epsilon = 0.1;
    let trial = Math.random();

    if(trial < epsilon / 2) {
        return 'on';
    } else if(trial < epsilon) {
        return 'off';
    } else {
        return est_value_on > est_value_off ? 'on' : 'off';
    }
};

let policy_refine = (policy, cur_action, cur_ball_pos, new_ball_pos, reward) => {
    let cur_est_value = cur_action === 'on'
        ? cosine_basis_value(policy.reward_fn_force_on, cur_ball_pos)
        : cosine_basis_value(policy.reward_fn_force_off, cur_ball_pos);


    let new_action = policy_apply(policy, new_ball_pos);
    let new_est_value = new_action === 'on'
        ? cosine_basis_value(policy.reward_fn_force_on, new_ball_pos)
        : cosine_basis_value(policy.reward_fn_force_off, new_ball_pos);

    let delta = reward - policy.average_reward + new_est_value - cur_est_value;

    if(cur_action === 'on') {
        cosine_basis_refine(policy.reward_fn_force_on,
                            cur_ball_pos,
                            delta,
                            policy.step_alpha);
    } else {
        cosine_basis_refine(policy.reward_fn_force_off,
                            cur_ball_pos,
                            delta,
                            policy.step_alpha);
    }

    policy.average_reward += policy.step_beta * delta;

    return new_action;
};

let policy_visualize = (cnv) => {
    let ctx = cnv.getContext('2d');

    let chart = new Chart(ctx,
                          {type: 'scatter',
                           data: {datasets: [{label: 'Action-Value Function: On',
                                              borderColor: 'red',
                                              data: [],
                                              fill: false,
                                              lineTension: 0,
                                              radius: 0},
                                             {label: 'Action-Value Function: Off',
                                              borderColor: 'blue',
                                              data: [],
                                              fill: false,
                                              lineTension: 0,
                                              radius: 0}]},
                           options: {scales: {xAxes: [{ticks: {suggestedMin: 0,
                                                               suggestedMax: 1.0}}]},
                                     showLines: true}});

    let lerp_foreach = (min, max, n, fn) => {
        for(let i = 0; i <= (n-1); ++i) {
            let x = min * ((n-1) - i) / (n-1) + max * i / (n-1);
            fn(x, i);
        }
    };
    
    let render_fn = (basis, n_points, data_out) => {
        if(data_out.length != n_points) {
            data_out.length = 0;
            lerp_foreach(basis.min_x,
                         basis.max_x,
                         n_points,
                         (x, i) => {data_out.push({x: x,
                                                   y: cosine_basis_value(basis, x)});});
        } else {
            lerp_foreach(basis.min_x,
                         basis.max_x,
                         n_points,
                         (x, i) => {data_out[i].x = x;
                                    data_out[i].y = cosine_basis_value(basis, x)});
        }
    };

    return (policy) => {
        render_fn(policy.reward_fn_force_on, 256, chart.data.datasets[0].data);
        render_fn(policy.reward_fn_force_off, 256, chart.data.datasets[1].data);
        chart.update(0);
    };
};

let circ_buf_make = (n) => {
    return {buf: new Float32Array(n),
            src: 0,
            len: 0};
};

let circ_buf_append = (cbuf, x) => {
    let mod = (i) => (i % cbuf.buf.length);

    if(cbuf.len === cbuf.buf.length) {
        cbuf.buf[mod(cbuf.src + cbuf.len)] = x;
        cbuf.src = mod(cbuf.src + 1);
    } else {
        cbuf.buf[mod(cbuf.src + cbuf.len)] = x;
        cbuf.len = cbuf.len + 1;
    }
};

let circ_buf_at = (cbuf, i) => {
    let mod = (j) => (j % cbuf.buf.length);
    return cbuf.buf[mod(cbuf.src + i)];
};

let reward_trace_visualize = (cnv, n) => {
    let ctx = cnv.getContext('2d');

    let data = [];
    
    let chart = new Chart(ctx,
                          {type: 'scatter',
                           data: {datasets: [{label: 'Per-Episode Reward Trace',
                                              borderColor: 'green',
                                              data: data,
                                              fill: false,
                                              lineTension: 0,
                                              radius: 0}]},
                           options: {scales: {xAxes: [{ticks: {suggestedMin: 0,
                                                               suggestedMax: 1.0}}]},
                                     showLines: true}});

    let x_circ_buf = circ_buf_make(1000);
    let y_circ_buf = circ_buf_make(1000);

    return (trial_num, reward) => {
        circ_buf_append(x_circ_buf, trial_num);
        circ_buf_append(y_circ_buf, reward);

        while(data.length < x_circ_buf.len) {
            data.push({x: 0, y: 0});
        }
        
        for(let i = 0; i < x_circ_buf.len; ++i) {
            data[i].x = circ_buf_at(x_circ_buf, i);
            data[i].y = circ_buf_at(y_circ_buf, i);
        }
        
        chart.update(0);
    };
};

let state_visualize = (cnv) => {
    let ctx = cnv.getContext('2d');

    return (cur_ball, agent_force) => {
        ctx.clearRect(0, 0, cnv.getWidth, cnv.getHeight);

        
    };
};

let main = () => {
    let qs = x => document.querySelector(x);

    let bind_float = (elt, fn) => {
        let loader = () => {
            let val = parseFloat(elt.value);
            if(!isNaN(val)) {
                fn(val);
            }
        }

        loader();
        elt.addEventListener('change', (event) => {loader();});
    };

    let sim_timescale = 1.0;
    bind_float(qs('#input-timescale'), v => {sim_timescale = v;});

    let learn_period = 0.01;
    bind_float(qs('#input-learn-frequency'), v => {learn_period = 1.0 / v;});
    
    let state_canvas = qs('#state-figure-canvas');
    let state_canvas_ctx = state_canvas.getContext('2d');

    let policy_visualizer = policy_visualize(qs('#action-value-function-canvas'));

    let reward_trace_visualizer = reward_trace_visualize(qs('#episodic-reward-trace-canvas'));
    
    let ball_factory = () => ball_make_random(0.1, 0.2, 0.8);
    let ball = ball_factory();

    let policy = policy_make(64, 0.05, 0.0001, 0.0001);
    
    let prev_ball_pos = ball.pos;
    let cur_action = policy_apply(policy, ball.pos);
    let agent_force = (action) => action === 'on' ? 2.0 : 0.0;

    let trial_num = 0;
    let trial_reward_sum = 0;
    let trial_learn_frames = 0;
    let trial_phys_frames = 0;
    let learn_period_reward_sum = 0;

    // The system dynamics are always simulated with a fixed timestep,
    // iterated enough times to bring us up to the current simulation
    // time.
    let physics_dt = 0.0001;
    
    let cur_time = performance.now() / 1000.0;
    let sim_cur_time = 0;
    let phys_cur_time = 0;
    let learn_last_time = 0;
    let last_reset_time = 0;
    let real_time_step = (new_time) => {
        new_time /= 1000.0;

        let delta = new_time - cur_time;

        // Clamp our elapsed real time, to prevent a long first frame after
        // pausing in the debugger.
        if(delta > 0.1) {
            delta = 0.1;
        }
        
        let sim_delta = delta * sim_timescale;
        let sim_new_time = sim_cur_time + sim_delta;

        while(phys_cur_time < sim_new_time) {
            ball_integrate(ball, agent_force(cur_action), physics_dt);
            phys_cur_time += physics_dt;

            let step_reward = reward(ball, physics_dt);
            learn_period_reward_sum += step_reward;
            trial_reward_sum += step_reward;
            
            // If we've lived long enough, end the episode.
            if(phys_cur_time - last_reset_time > 1000.0) {
                last_reset_time = phys_cur_time;
                learn_last_time = phys_cur_time;
                policy_refine(policy,
                              cur_action,
                              prev_ball_pos,
                              ball.pos,
                              0.0);
                ball = ball_factory();
                prev_ball_pos = ball.pos;
                cur_action = policy_apply(policy, ball);

                trial_reward_sum += 0.0;
                reward_trace_visualizer(trial_num, trial_reward_sum);
                trial_num += 1;
                trial_reward_sum = 0.0;
                learn_period_reward_sum = 0.0;
            }

            // If the ball has gone out of bounds, end the episode.
            if(ball.pos < 0.0 || ball.pos > 1.0) {  
                last_reset_time = phys_cur_time;
                learn_last_time = phys_cur_time;
                policy_refine(policy,
                              cur_action,
                              prev_ball_pos,
                              ball.pos,
                              -1000.0);
                ball = ball_factory();
                prev_ball_pos = ball.pos;
                cur_action = policy_apply(policy, ball);

                trial_reward_sum += -1000.0;
                reward_trace_visualizer(trial_num, trial_reward_sum);
                trial_num += 1;
                trial_reward_sum = 0.0;
                learn_period_reward_sum = 0.0;
            }

            // If the learning period has elapsed, refine the policy.
            if(phys_cur_time > learn_last_time + learn_period) {
                trial_learn_frames += 1;         
                
                learn_last_time = phys_cur_time;
                cur_action = policy_refine(policy,
                                           cur_action,
                                           prev_ball_pos,
                                           ball.pos,
                                           learn_period_reward_sum);

                learn_period_reward_sum = 0;
            }

            trial_phys_frames += 1;
        }

        policy_visualizer(policy);

        cur_time = new_time;
        sim_cur_time = sim_new_time;

        state_canvas_ctx.clearRect(0, 0, state_canvas.width, state_canvas.height);
        state_canvas_ctx.save();
        state_canvas_ctx.translate(state_canvas.width / 2,
                                   state_canvas.height / 2);
        state_canvas_ctx.scale(1.0, -1.0);

        // Draw ruler
        state_canvas_ctx.strokeStyle = 'black';
        state_canvas_ctx.beginPath();
        state_canvas_ctx.moveTo(-20, 0);
        state_canvas_ctx.lineTo(-20, 100);
        state_canvas_ctx.stroke();

        // Draw agent force indicator
        state_canvas_ctx.save();
        state_canvas_ctx.translate(50, 0);

        state_canvas_ctx.strokeStyle = 'black';
        state_canvas_ctx.beginPath();
        state_canvas_ctx.moveTo(-10.0, -20.0);
        state_canvas_ctx.lineTo(-10.0,  20.0);
        state_canvas_ctx.stroke();

        state_canvas_ctx.fillStyle = 'green';
        state_canvas_ctx.fillRect(-5.0, 0.0, 10.0, agent_force(cur_action));

        state_canvas_ctx.restore();

        // Draw current position of the ball.
        state_canvas_ctx.fillStyle = 'red';
        state_canvas_ctx.fillRect(-10, ball.pos * 100 - 10, 20, 20);

        state_canvas_ctx.restore();

        window.requestAnimationFrame(real_time_step);
    };

    window.requestAnimationFrame(real_time_step);

};

document.addEventListener('DOMContentLoaded', (event) => main());



