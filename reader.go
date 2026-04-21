package main

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
)

type Pool struct {
	alloc  context.Context
	cancel context.CancelFunc
	ch     chan context.Context
}

func NewPool(size int) *Pool {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)

	p := &Pool{
		alloc:  alloc,
		cancel: cancel,
		ch:     make(chan context.Context, size),
	}

	for i := 0; i < size; i++ {
		ctx, _ := chromedp.NewContext(alloc)
		p.ch <- ctx
	}

	return p
}

func (p *Pool) Get() context.Context {
	return <-p.ch
}

func (p *Pool) Put(ctx context.Context) {
	p.ch <- ctx
}

func (p *Pool) Fetch(url string) (string, error) {
	ctx := p.Get()
	defer p.Put(ctx)

	var html string

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	)

	return html, err
}
