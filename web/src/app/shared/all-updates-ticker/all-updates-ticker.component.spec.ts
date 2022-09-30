import { ComponentFixture, TestBed, waitForAsync } from '@angular/core/testing';

import { AllEnabledCurrencyTickersComponent } from './all-updates-ticker.component';

describe('AllEnabledCurrencyTickersComponent', () => {
  let component: AllEnabledCurrencyTickersComponent;
  let fixture: ComponentFixture<AllEnabledCurrencyTickersComponent>;

  beforeEach(waitForAsync(() => {
    TestBed.configureTestingModule({
      declarations: [ AllEnabledCurrencyTickersComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(AllEnabledCurrencyTickersComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should be created', () => {
    expect(component).toBeTruthy();
  });
});
